package config

import (
	"gopkg.in/go-playground/validator.v9"
	"gopkg.in/yaml.v2"
	"io/ioutil"
)

type Config struct {
	ServiceAddr string `yaml:"service_addr"`

	Domain string `yaml:"domain" validate:"required"`

	IMAP *Server `yaml:"imap"`
	SMTP *Server `yaml:"smtp"`
}

type Server struct {
	Host     string `yaml:"server" validate:"required"`
	Port     int    `yaml:"port" validate:"required"`
	STARTTLS bool   `yaml:"starttls"`
}

func NewConfig(p string) (*Config, error) {
	b, err := ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}

	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}

	validate := validator.New()
	if err := validate.Struct(c); err != nil {
		return nil, err
	}

	return &c, nil
}
