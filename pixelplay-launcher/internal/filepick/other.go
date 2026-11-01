//go:build !linux

package filepick

func Open(title, filter string) (string, error) {
	return "", ErrUnsupported
}

func OpenMany(title, filter string) ([]string, error) {
	return nil, ErrUnsupported
}

func OpenDir(title string) (string, error) {
	return "", ErrUnsupported
}

func Save(title, suggested, filter string) (string, error) {
	return "", ErrUnsupported
}
