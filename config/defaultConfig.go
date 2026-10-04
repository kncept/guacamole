package config

import (
	"errors"
	"os"
	"path/filepath"
)

// GuacDir returns guacamole's state directory: ~/.guac. Sessions and
// permissions live under it.
func GuacDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".guac"), nil
}

type ApiModelInterfaceDetails struct {
	BaseUrl   string
	ApiKey    string
	ModelName string
}

func DefaultConfig() (*ApiModelInterfaceDetails, error) {
	var config *ApiModelInterfaceDetails
	var err error
	config, err = OpenCodeConfig()
	if err != nil {
		panic(err)
	}
	if config != nil {
		return config, nil
	}

	return nil, errors.ErrUnsupported
}
