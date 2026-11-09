package assets

import "embed"

//go:embed fonts/Sarabun-Regular.ttf fonts/Sarabun-Bold.ttf
var FontFS embed.FS

func SarabunRegular() ([]byte, error) {
	return FontFS.ReadFile("fonts/Sarabun-Regular.ttf")
}

func SarabunBold() ([]byte, error) {
	return FontFS.ReadFile("fonts/Sarabun-Bold.ttf")
}
