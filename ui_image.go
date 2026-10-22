package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// uiImage is a picture file for the page: its properties and, if it is small enough, the picture
// itself as a data URL (the page draws it in an <img>, where an SVG cannot run scripts).
type uiImage struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Mime     string `json:"mime"`
	Size     int64  `json:"size"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Modified string `json:"modified"`
	Data     string `json:"data,omitempty"`
	Err      string `json:"err,omitempty"`
}

const uiImageMax = 8 << 20

var imageTypes = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif",
	".webp": "image/webp", ".bmp": "image/bmp", ".ico": "image/x-icon", ".avif": "image/avif",
	".svg": "image/svg+xml",
}

var svgSize = regexp.MustCompile(`(?s)<svg[^>]*?(?:\swidth="([\d.]+)(?:px)?")[^>]*?(?:\sheight="([\d.]+)(?:px)?")`)
var svgBox = regexp.MustCompile(`viewBox="[\d.\-]+[ ,]+[\d.\-]+[ ,]+([\d.]+)[ ,]+([\d.]+)"`)

func uiImageOf(path string) uiImage {
	img := uiImage{Path: path, Name: filepath.Base(path)}
	fi, err := os.Stat(path)
	if err != nil {
		img.Err = err.Error()
		return img
	}
	img.Size, img.Modified = fi.Size(), ago(fi.ModTime())
	ext := strings.ToLower(filepath.Ext(path))
	img.Mime = imageTypes[ext]
	if img.Mime == "" {
		img.Mime = mime.TypeByExtension(ext)
	}
	if fi.IsDir() || img.Mime == "" || !strings.HasPrefix(img.Mime, "image/") {
		img.Err = "not a picture"
		return img
	}
	if fi.Size() > uiImageMax {
		return img // too big to ship through the page: the properties only
	}
	b, err := os.ReadFile(path)
	if err != nil {
		img.Err = err.Error()
		return img
	}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(b)); err == nil {
		img.Width, img.Height = cfg.Width, cfg.Height
	} else if img.Mime == "image/svg+xml" {
		img.Width, img.Height = svgDimensions(string(b))
	}
	img.Data = fmt.Sprintf("data:%s;base64,%s", img.Mime, base64.StdEncoding.EncodeToString(b))
	return img
}

// svgDimensions reads the size an SVG declares: width and height, or else its viewBox.
func svgDimensions(src string) (int, int) {
	head := src
	if len(head) > 4096 {
		head = head[:4096]
	}
	if m := svgSize.FindStringSubmatch(head); m != nil {
		w, _ := strconv.ParseFloat(m[1], 64)
		h, _ := strconv.ParseFloat(m[2], 64)
		return int(w), int(h)
	}
	if m := svgBox.FindStringSubmatch(head); m != nil {
		w, _ := strconv.ParseFloat(m[1], 64)
		h, _ := strconv.ParseFloat(m[2], 64)
		return int(w), int(h)
	}
	return 0, 0
}
