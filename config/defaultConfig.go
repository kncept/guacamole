package config

import "errors"

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
