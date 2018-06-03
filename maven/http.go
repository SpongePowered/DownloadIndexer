package maven

import (
	"github.com/SpongePowered/DownloadIndexer/httperror"
	"io"
	"net/http"
	"net/url"
	"os"
)

func createHTTP(url *url.URL) (*httpRepository, error) {
	repo := &httpRepository{user: url.User}
	url.User = nil
	repo.url = url.String()

	durl, ok := os.LookupEnv("HTTP_DOWNLOAD_URL")
	if ok {
		repo.downloadURL = durl
	} else {
		repo.downloadURL = repo.url
	}

	return repo, nil
}

type httpRepository struct {
	url         string
	downloadURL string
	user        *url.Userinfo
}

func (repo *httpRepository) prepareRequest(method string, path string, body io.Reader) (req *http.Request, err error) {
	req, err = http.NewRequest(method, path, body)
	if err != nil {
		return
	}

	// Setup authentication
	if repo.user != nil {
		password, _ := repo.user.Password()
		req.SetBasicAuth(repo.user.Username(), password)
	}

	return
}

func doRequest(req *http.Request) (resp *http.Response, err error) {
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		err = httperror.New(http.StatusBadGateway, "Failed to contact upstream server", err)
	}
	return
}

func (repo *httpRepository) Download(path string, writer io.Writer) error {
	req, err := repo.prepareRequest(http.MethodGet, repo.downloadURL+path, nil)
	if err != nil {
		return err
	}

	resp, err := doRequest(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		io.Copy(writer, resp.Body)
		return nil
	}

	return httperror.New(resp.StatusCode, "Failed to download file", nil)
}

func (repo *httpRepository) Upload(path string, reader io.Reader, len int64) error {
	req, err := repo.prepareRequest(http.MethodPut, repo.url+path, reader)
	if err != nil {
		return err
	}

	req.ContentLength = len

	resp, err := doRequest(req)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	return httperror.New(resp.StatusCode, "Failed to upload file", nil)
}
