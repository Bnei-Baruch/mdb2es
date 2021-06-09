package es

import (
	"code.sajari.com/docconv"
	"github.com/pkg/errors"
)

func ParseDocx(docxPath string) (string, error) {
	res, err := docconv.ConvertPath(docxPath)
	if err != nil {
		return "", errors.Wrap(err, "docconv.ConvertPath")
	}
	return res.Body, nil
}
