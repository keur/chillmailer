package util

import (
	"errors"
	"fmt"
	"net/mail"
	"os"
	"regexp"
	"strings"
)

func StringIsNo(s string) bool {
	n := strings.ToLower(s)
	if n == "" || n == "n" || n == "no" || n == "0" || n == "f" || n == "false" {
		return true
	} else {
		return false
	}
}

func StringIsYes(s string) bool {
	return !StringIsNo(s)
}

func GetenvOr(s string, fallback string) string {
	r := os.Getenv(s)
	if r == "" {
		return fallback
	}
	return r
}

func GetenvOrError(s string) (string, error) {
	r := os.Getenv(s)
	if r == "" {
		return "", errors.New(fmt.Sprintf("Required environment variable %s", s))
	}
	return r, nil
}

func GetEnvOrPanic(s string) string {
	e, err := GetenvOrError(s)
	if err != nil {
		panic(err)
	}
	return e
}

func IsEmailValid(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

var WhitespaceRegexp = regexp.MustCompile(`\s`)

func ReplaceWhitespaceWith(s string, rep string) string {
	return WhitespaceRegexp.ReplaceAllString(s, rep)
}
