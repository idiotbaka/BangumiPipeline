package imageutil

import (
	"fmt"
	"image"
	"image/jpeg"
	"os"

	_ "image/png"
)

func EncodeJPEG(sourcePath, destinationPath string, quality int) error {
	input, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer input.Close()

	img, _, err := image.Decode(input)
	if err != nil {
		return fmt.Errorf("解析图片失败: %w", err)
	}
	output, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	encodeErr := jpeg.Encode(output, img, &jpeg.Options{Quality: quality})
	closeErr := output.Close()
	if encodeErr != nil {
		return fmt.Errorf("写入 JPG 图片失败: %w", encodeErr)
	}
	return closeErr
}
