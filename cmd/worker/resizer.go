package main

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"

	"github.com/disintegration/imaging"
)

type ImageResizer struct{}

func NewImageResizer() *ImageResizer {
	return &ImageResizer{}
}

func (r *ImageResizer) Resize(original io.Reader, width, height int, format imaging.Format) ([]byte, error) {
	img, _, err := image.Decode(original)
	if err != nil {
		return nil, err
	}

	resized := imaging.Resize(img, width, height, imaging.Lanczos)

	buf := new(bytes.Buffer)

	switch format {
	case imaging.JPEG:
		err = imaging.Encode(buf, resized, imaging.JPEG)
		if err != nil {
			return nil, fmt.Errorf("failed to encode JPEG: %w", err)
		}
	case imaging.PNG:
		err = imaging.Encode(buf, resized, imaging.PNG)
		if err != nil {
			return nil, fmt.Errorf("failed to encode PNG: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported format: %v", format)
	}

	return buf.Bytes(), nil
}
