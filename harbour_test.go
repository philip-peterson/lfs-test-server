package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"testing"
	"time"
)

var (
	testMetaStore  *MetaStore
	testUser       = "bilbo"
	testPass       = "baggins"
	authedOid      = "44ce7dd67c959e0d3524ffac1771dfbba87d2b6b4b4e99e42034a8b803f8b072"
	nonexistingOid = "aec070645fe53ee3b3763059376134f058cc337247c978add178b6ccdfb0019f"
)

func TestMain(m *testing.M) {
	os.Remove("lfs-test.db")

	var err error
	testMetaStore, err = NewMetaStore("lfs-test.db")
	if err != nil {
		os.Exit(1)
	}

	if err := seedMetaStore(); err != nil {
		fmt.Printf("Error seeding meta store: %s", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func seedMetaStore() error {
	if err := testMetaStore.AddUser(testUser, testPass); err != nil {
		return err
	}

	rv := &RequestVars{User: testUser, Password: testPass, Oid: authedOid, Size: 1234}
	if _, err := testMetaStore.Put(rv); err != nil {
		return err
	}

	return nil
}

func TestGetAuthed(t *testing.T) {
	testSetup()
	defer testTeardown()

	req, err := http.NewRequest("GET", mediaServer.URL+"/user/repo/objects/"+authedOid, nil)
	if err != nil {
		t.Fatalf("request error: %s", err)
	}
	req.SetBasicAuth(testUser, testPass)
	req.Header.Set("Accept", contentMediaType)

	res, err := http.DefaultTransport.RoundTrip(req) // Do not follow the redirect
	if err != nil {
		t.Fatalf("response error: %s", err)
	}

	if res.StatusCode != 302 {
		t.Fatalf("expected status 302, got %d", res.StatusCode)
	}
}

func TestGetUnauthed(t *testing.T) {
	testSetup()
	defer testTeardown()

	req, err := http.NewRequest("GET", mediaServer.URL+"/user/repo/objects/"+authedOid, nil)
	if err != nil {
		t.Fatalf("request error: %s", err)
	}
	req.Header.Set("Accept", contentMediaType)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("response error: %s", err)
	}

	if res.StatusCode != 404 {
		t.Fatalf("expected status 404, got %d %s", res.StatusCode, req.URL)
	}
}

func TestGetMetaAuthed(t *testing.T) {
	testSetup()
	defer testTeardown()

	req, err := http.NewRequest("GET", mediaServer.URL+"/bilbo/repo/objects/"+authedOid, nil)
	if err != nil {
		t.Fatalf("request error: %s", err)
	}
	req.SetBasicAuth(testUser, testPass)
	req.Header.Set("Accept", metaMediaType)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("response error: %s", err)
	}

	if res.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d %s", res.StatusCode, req.URL)
	}

	var meta Representation
	dec := json.NewDecoder(res.Body)
	dec.Decode(&meta)

	if meta.Oid != authedOid {
		t.Fatalf("expected to see oid `%s` in meta, got: `%s`", authedOid, meta.Oid)
	}

	if meta.Size != 1234 {
		t.Fatalf("expected to see a size of `1234`, got: `%d`", meta.Size)
	}

	download := meta.Links["download"]
	if download.Href != "http://localhost:8080/bilbo/repo/objects/"+authedOid {
		t.Fatalf("expected download link, got %s", download.Href)
	}
}

func TestGetMetaUnauthed(t *testing.T) {
	testSetup()
	defer testTeardown()

	req, err := http.NewRequest("GET", mediaServer.URL+"/user/repo/objects/"+authedOid, nil)
	if err != nil {
		t.Fatalf("request error: %s", err)
	}
	req.Header.Set("Accept", metaMediaType)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("response error: %s", err)
	}

	if res.StatusCode != 401 {
		t.Fatalf("expected status 401, got %d", res.StatusCode)
	}
}

func TestPostAuthedNewObject(t *testing.T) {
	testSetup()
	defer testTeardown()

	req, err := http.NewRequest("POST", mediaServer.URL+"/bilbo/repo/objects", nil)
	if err != nil {
		t.Fatalf("request error: %s", err)
	}
	req.SetBasicAuth(testUser, testPass)
	req.Header.Set("Accept", metaMediaType)

	buf := bytes.NewBufferString(fmt.Sprintf(`{"oid":"%s", "size":1234}`, nonexistingOid))
	req.Body = ioutil.NopCloser(buf)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("response error: %s", err)
	}

	if res.StatusCode != 201 {
		t.Fatalf("expected status 201, got %d", res.StatusCode)
	}

	var meta Representation
	dec := json.NewDecoder(res.Body)
	dec.Decode(&meta)

	if meta.Oid != nonexistingOid {
		t.Fatalf("expected to see oid `%s` in meta, got: `%s`", nonexistingOid, meta.Oid)
	}

	if meta.Size != 1234 {
		t.Fatalf("expected to see a size of `1234`, got: `%d`", meta.Size)
	}

	download := meta.Links["download"]
	if download.Href != "http://localhost:8080/bilbo/repo/objects/"+nonexistingOid {
		t.Fatalf("expected download link, got %s", download.Href)
	}

	upload, ok := meta.Links["upload"]
	if !ok {
		t.Fatal("expected upload link to be present")
	}

	if upload.Href != "https://examplebucket.s3.amazonaws.com"+oidPath(nonexistingOid) {
		t.Fatalf("expected upload link, got %s", upload.Href)
	}
}

func TestPostAuthedExistingObject(t *testing.T) {
	testSetup()
	defer testTeardown()

	req, err := http.NewRequest("POST", mediaServer.URL+"/bilbo/repo/objects", nil)
	if err != nil {
		t.Fatalf("request error: %s", err)
	}
	req.SetBasicAuth(testUser, testPass)
	req.Header.Set("Accept", metaMediaType)

	buf := bytes.NewBufferString(fmt.Sprintf(`{"oid":"%s", "size":1234}`, authedOid))
	req.Body = ioutil.NopCloser(buf)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("response error: %s", err)
	}

	if res.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	var meta Representation
	dec := json.NewDecoder(res.Body)
	dec.Decode(&meta)

	if meta.Oid != authedOid {
		t.Fatalf("expected to see oid `%s` in meta, got: `%s`", authedOid, meta.Oid)
	}

	if meta.Size != 1234 {
		t.Fatalf("expected to see a size of `1234`, got: `%d`", meta.Size)
	}

	download := meta.Links["download"]
	if download.Href != "http://localhost:8080/bilbo/repo/objects/"+authedOid {
		t.Fatalf("expected download link, got %s", download.Href)
	}

	upload, ok := meta.Links["upload"]
	if !ok {
		t.Fatalf("expected upload link to be present")
	}

	if upload.Href != "https://examplebucket.s3.amazonaws.com"+oidPath(authedOid) {
		t.Fatalf("expected upload link, got %s", upload.Href)
	}
}

func TestPostUnauthed(t *testing.T) {
	testSetup()
	defer testTeardown()

	req, err := http.NewRequest("POST", mediaServer.URL+"/bilbo/readonly/objects", nil)
	if err != nil {
		t.Fatalf("request error: %s", err)
	}
	req.Header.Set("Accept", metaMediaType)

	buf := bytes.NewBufferString(fmt.Sprintf(`{"oid":"%s", "size":1234}`, authedOid))
	req.Body = ioutil.NopCloser(buf)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("response error: %s", err)
	}

	if res.StatusCode != 401 {
		t.Fatalf("expected status 401, got %d", res.StatusCode)
	}
}

func TestPut(t *testing.T) {
	testSetup()
	defer testTeardown()

	req, err := http.NewRequest("PUT", mediaServer.URL+"/user/repo/objects/"+authedOid, nil)
	if err != nil {
		t.Fatalf("request error: %s", err)
	}
	req.Header.Set("Authorization", authedToken)
	req.Header.Set("Accept", contentMediaType)
	req.Header.Set("Content-Type", "application/octet-stream")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("response error: %s", err)
	}

	if res.StatusCode != 405 {
		t.Fatalf("expected status 405, got %d", res.StatusCode)
	}
}

func TestMediaTypesRequired(t *testing.T) {
	testSetup()
	defer testTeardown()

	m := []string{"GET", "PUT", "OPTIONS"}
	for _, method := range m {
		req, err := http.NewRequest(method, mediaServer.URL+"/user/repo/objects/"+authedOid, nil)
		if err != nil {
			t.Fatalf("request error: %s", err)
		}
		req.SetBasicAuth(testUser, testPass)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("response error: %s", err)
		}

		if res.StatusCode != 404 {
			t.Fatalf("expected status 404, got %d", res.StatusCode)
		}
	}
}

func TestMediaTypesParsed(t *testing.T) {
	testSetup()
	defer testTeardown()

	req, err := http.NewRequest("GET", mediaServer.URL+"/user/repo/objects/"+authedOid, nil)
	if err != nil {
		t.Fatalf("request error: %s", err)
	}
	req.SetBasicAuth(testUser, testPass)
	req.Header.Set("Accept", contentMediaType+"; charset=utf-8")

	res, err := http.DefaultTransport.RoundTrip(req) // Do not follow the redirect
	if err != nil {
		t.Fatalf("response error: %s", err)
	}

	if res.StatusCode != 302 {
		t.Fatalf("expected status 302, got %d", res.StatusCode)
	}
}

var (
	now         time.Time
	mediaServer *httptest.Server

	authedToken = "AUTHORIZED"
)

func testSetup() {
	Config.AwsKey = "AKIAIOSFODNN7EXAMPLE"
	Config.AwsSecretKey = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
	Config.AwsBucket = "examplebucket"
	Config.Scheme = "http"

	contentSha = sha256Hex([]byte(content))
	now, _ = time.Parse(time.RFC822, "24 May 13 00:00 GMT")

	app := NewApp(&TestRedirector{}, testMetaStore)
	mediaServer = httptest.NewServer(app.Router)

	logger = NewKVLogger(ioutil.Discard)
}

func testTeardown() {
	mediaServer.Close()
}

type TestRedirector struct {
}

func (t *TestRedirector) Get(meta *Meta, w http.ResponseWriter, r *http.Request) int {
	token := S3SignQuery("GET", path.Join("/", meta.PathPrefix, oidPath(meta.Oid)), 86400)
	w.Header().Set("Location", token.Location)
	w.WriteHeader(302)
	return 302
}

func (t *TestRedirector) PutLink(meta *Meta) *link {
	token := S3SignHeader("PUT", path.Join("/", meta.PathPrefix, oidPath(meta.Oid)), meta.Oid)
	header := make(map[string]string)
	header["Authorization"] = token.Token
	header["x-amz-content-sha256"] = meta.Oid
	header["x-amz-date"] = token.Time.Format(isoLayout)

	return &link{Href: token.Location, Header: header}
}

func (t *TestRedirector) Exists(*Meta) (bool, error) {
	return true, nil
}
