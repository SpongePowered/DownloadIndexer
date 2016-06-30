package maven

type Uploader interface {
	Upload(path string, data []byte) error
}

type Proxy struct {
	Target   string
	Uploader []Uploader
}

func (p *Proxy) Get(path string) string {
	return p.Target + path
}

func (p *Proxy) Upload(path string, data []byte) (err error) {
	for _, uploader := range p.Uploader {
		err = uploader.Upload(path, data)
		if err != nil {
			return
		}
	}

	return
}
