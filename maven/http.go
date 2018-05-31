package maven

import (
	"github.com/SpongePowered/DownloadIndexer/httperror"
	"io"
	"net/http"
	"net/url"
)

func createHTTP(url *url.URL) (*httpRepository, error) {
	repo := &httpRepository{user: url.User}
	url.User = nil
	repo.url = url.String()
	return repo, nil
}

type httpRepository struct {
	url  string
	user *url.Userinfo
}

func (repo *httpRepository) runRequest(method string, path string, body io.Reader) (resp *http.Response, err error) {
	req, err := http.NewRequest(method, repo.url+path, nil)
	if err != nil {
		return
	}

	// Setup authentication
	if repo.user != nil {
		password, _ := repo.user.Password()
		req.SetBasicAuth(repo.user.Username(), password)
	}

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		err = httperror.New(http.StatusBadGateway, "Failed to contact upstream server", err)
	}
	return
}

func (repo *httpRepository) Download(path string, writer io.Writer) error {
	resp, err := repo.runRequest(http.MethodGet, path, nil)
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

func (repo *httpRepository) Upload(path string, reader io.Reader) error {
	resp, err := repo.runRequest(http.MethodPut, path, reader)
	if err != nil {
		return err
	}

	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	return httperror.New(resp.StatusCode, "Failed to upload file", nil)
}
