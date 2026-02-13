package media

import basemedia "github.com/beeper/ai-bridge/pkg/shared/media"

func ParseDataURI(dataURI string) (string, string, error) {
	return basemedia.ParseDataURI(dataURI)
}

func DecodeBase64(b64Data string) ([]byte, string, error) {
	return basemedia.DecodeBase64(b64Data)
}
