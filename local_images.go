package main

import (
	"image"
	"math/rand"

	log "github.com/sirupsen/logrus"

	"github.com/disintegration/imaging"
)

// LocalImagesConfig supports a single dest and a list of image to randomly publish to that topic
type LocalImagesConfig struct {
	Topic   string       `yaml:"topic"`
	Jobs    JobRunnerCfg `yaml:"jobs"`
	Height  int          `yaml:"height"`
	Width   int          `yaml:"width"`
	Sources []string     `yaml:"sources"`
}

func initLocalImages(bmux BrokerMux, cfg LocalImagesConfig) {
	if len(cfg.Topic) > 0 && len(cfg.Sources) > 0 {
		NewJobRunner("images-local", cfg.Jobs, func() {
			index := rand.Intn(len(cfg.Sources))
			publishLocalImage(bmux, cfg.Topic, cfg.Sources[index], cfg.Height, cfg.Width)
		}).Run()
	}
}

// ImageToMatrixBytes turns a NRGBA image into an array of 3 byte pixels
func ImageToMatrixBytes(img *image.NRGBA) []byte {

	maxX := img.Bounds().Max.X
	maxY := img.Bounds().Max.Y

	// Create the raw byte output.  Hope the RGBs line up
	output := make([]byte, maxX*maxY*3)
	index := 0
	for y := 0; y < maxY; y++ {
		for x := 0; x < maxX; x++ {
			rt, gt, bt, _ := img.At(x, y).RGBA()
			output[index+0] = byte(rt)
			output[index+1] = byte(gt)
			output[index+2] = byte(bt)
			index = index + 3
		}
	}
	return output
}

func publishLocalImage(bmux BrokerMux, topic string, source string, height int, width int) {
	img, err := imaging.Open(source)
	if err != nil {
		log.Errorf("images-local: %s: open: %v", source, err)
		return
	}

	final := imaging.Resize(img, width, height, imaging.Lanczos)
	if err != nil {
		log.Errorf("images-local: %s: resize: %v", source, err)
		return
	}

	log.Infof("images-local: posting %s to %s", source, topic)
	bmux.Publish(topic, 0, false, ImageToMatrixBytes(final))
}
