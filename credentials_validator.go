package main

import "errors"

var (
	errorInvalidCreds = errors.New("credentials invalid")
	errorNoMatch      = errors.New("no credentials in content")
)

type CredentialValidator interface {
	Match(content string) ([]CloudCredentials, error)
	Validate(CloudCredentials) bool
}

func ParseCredentials(content string, cv CredentialValidator) ([]CloudCredentials, error) {
	res := []CloudCredentials{}
	cc, err := cv.Match(content)
	if err != nil {
		return nil, errorNoMatch
	}

	for _, c := range cc {
		if cv.Validate(c) {
			res = append(res, c)
		}
	}

	return res, nil
}
