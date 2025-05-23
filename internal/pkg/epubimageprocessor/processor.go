// Package epubimageprocessor extract and transform image into a compressed jpeg.
package epubimageprocessor

import (
	"image"
	"image/color"
	"image/draw"
	"sync"

	"github.com/disintegration/gift"

	"github.com/ppkhoa/go-comic-converter/v3/internal/pkg/epubimage"
	"github.com/ppkhoa/go-comic-converter/v3/internal/pkg/epubimagefilters"
	"github.com/ppkhoa/go-comic-converter/v3/internal/pkg/epubprogress"
	"github.com/ppkhoa/go-comic-converter/v3/internal/pkg/epubzip"
	"github.com/ppkhoa/go-comic-converter/v3/internal/pkg/utils"
	"github.com/ppkhoa/go-comic-converter/v3/pkg/epuboptions"
)

type EPUBImageProcessor interface {
	Load() (images []epubimage.EPUBImage, err error)
	CoverTitleData(o CoverTitleDataOptions) (epubzip.Image, error)
}

type ePUBImageProcessor struct {
	epuboptions.EPUBOptions
}

func New(o epuboptions.EPUBOptions) EPUBImageProcessor {
	return ePUBImageProcessor{o}
}

// Load extract and convert images
func (e ePUBImageProcessor) Load() (images []epubimage.EPUBImage, err error) {
	images = make([]epubimage.EPUBImage, 0)
	imageCount, imageInput, err := e.load()
	if err != nil {
		return nil, err
	}

	// dry run, skip conversion
	if e.Dry {
		for img := range imageInput {
			images = append(images, epubimage.EPUBImage{
				Id:     img.Id,
				Path:   img.Path,
				Name:   img.Name,
				Format: e.Image.Format,
			})
		}

		return images, nil
	}

	imageOutput := make(chan epubimage.EPUBImage)

	// processing
	bar := epubprogress.New(epubprogress.Options{
		Quiet:       e.Quiet,
		Json:        e.Json,
		Max:         imageCount,
		Description: "Processing",
		CurrentJob:  1,
		TotalJob:    2,
	})
	wg := &sync.WaitGroup{}

	imgStorage, err := epubzip.NewStorageImageWriter(e.ImgStorage(), e.Image.Format)
	if err != nil {
		_ = bar.Close()
		return nil, err
	}

	wr := 50
	if e.Image.Format == "png" {
		wr = 100
	}
	for range e.WorkersRatio(wr) {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for input := range imageInput {
				img := e.transformImage(input, 0, e.Image.Manga)

				// do not keep double page if requested
				if !(img.DoublePage && input.Id > 0 &&
					e.EPUBOptions.Image.AutoSplitDoublePage && !e.EPUBOptions.Image.KeepDoublePageIfSplit) {
					if err = imgStorage.Add(img.EPUBImgPath(), img.Raw, e.Image.Quality); err != nil {
						_ = bar.Close()
						utils.Fatalf("error with %s: %s", input.Name, err)
					}
					// do not keep raw image except for cover
					if img.Id > 0 {
						img.Raw = nil
					}
					imageOutput <- img
				}

				// DOUBLE PAGE
				if !e.Image.AutoSplitDoublePage || // No split required
					!img.DoublePage || // Not a double page
					(e.Image.HasCover && img.Id == 0) { // Cover
					continue
				}

				for i, b := range []bool{e.Image.Manga, !e.Image.Manga} {
					img = e.transformImage(input, i+1, b)
					if err = imgStorage.Add(img.EPUBImgPath(), img.Raw, e.Image.Quality); err != nil {
						_ = bar.Close()
						utils.Fatalf("error with %s: %s", input.Name, err)
					}
					img.Raw = nil
					imageOutput <- img
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		_ = imgStorage.Close()
		close(imageOutput)
	}()

	for img := range imageOutput {
		if img.Part == 0 {
			_ = bar.Add(1)
		}
		if e.Image.NoBlankImage && img.IsBlank {
			continue
		}
		images = append(images, img)
	}
	_ = bar.Close()

	if len(images) == 0 {
		return nil, errNoImagesFound
	}

	return images, nil
}

func (e ePUBImageProcessor) createImage(src image.Image, r image.Rectangle) draw.Image {
	if e.EPUBOptions.Image.GrayScale {
		return image.NewGray(r)
	}

	switch t := src.(type) {
	case *image.Gray:
		return image.NewGray(r)
	case *image.Gray16:
		return image.NewGray16(r)
	case *image.RGBA:
		return image.NewRGBA(r)
	case *image.RGBA64:
		return image.NewRGBA64(r)
	case *image.NRGBA:
		return image.NewNRGBA(r)
	case *image.NRGBA64:
		return image.NewNRGBA64(r)
	case *image.Alpha:
		return image.NewAlpha(r)
	case *image.Alpha16:
		return image.NewAlpha16(r)
	case *image.CMYK:
		return image.NewCMYK(r)
	case *image.Paletted:
		return image.NewPaletted(r, t.Palette)
	default:
		return image.NewNRGBA64(r)
	}
}

// transform image into 1 or 3 images
// only doublepage with autosplit has 3 versions
func (e ePUBImageProcessor) transformImage(input task, part int, right bool) epubimage.EPUBImage {
	g := gift.New()
	src := input.Image
	srcBounds := src.Bounds()

	// In portrait only, we don't need to keep aspect ratio between each split.
	// We first cut, the crop.
	if part > 0 && !e.Image.KeepSplitDoublePageAspect {
		g.Add(epubimagefilters.CropSplitDoublePage(right))
	}

	// Lookup for margin if crop is enable or if we want to remove blank image
	if e.Image.Crop.Enabled || e.Image.NoBlankImage {
		f := epubimagefilters.AutoCrop(
			src,
			g.Bounds(src.Bounds()),
			e.Image.Crop.Left,
			e.Image.Crop.Up,
			e.Image.Crop.Right,
			e.Image.Crop.Bottom,
			e.Image.Crop.Limit,
			e.Image.Crop.SkipIfLimitReached,
		)

		// detect if blank image
		size := f.Bounds(srcBounds)
		isBlank := size.Dx() == 0 && size.Dy() == 0

		// crop is enable or if blank image with noblankimage options
		if e.Image.Crop.Enabled || (e.Image.NoBlankImage && isBlank) {
			g.Add(f)
		}
	}

	// With landscape support, we need to keep aspect ratio between each split
	// We first crop, then cut
	if part > 0 && e.Image.KeepSplitDoublePageAspect {
		g.Add(epubimagefilters.CropSplitDoublePage(right))
	}

	dstBounds := g.Bounds(src.Bounds())
	// Original && Cropped version need to landscape oriented
	// Only part 0 can be a double page
	isDoublePage := part == 0 && srcBounds.Dx() > srcBounds.Dy() && dstBounds.Dx() > dstBounds.Dy()

	if e.Image.AutoRotate && isDoublePage {
		g.Add(gift.Rotate90())
	}

	if e.Image.AutoContrast {
		g.Add(epubimagefilters.AutoContrast())
	}

	if e.Image.Contrast != 0 {
		g.Add(gift.Contrast(float32(e.Image.Contrast)))
	}

	if e.Image.Brightness != 0 {
		g.Add(gift.Brightness(float32(e.Image.Brightness)))
	}

	if e.Image.Resize {
		g.Add(gift.ResizeToFit(e.Image.View.Width, e.Image.View.Height, gift.LanczosResampling))
	}

	if e.Image.GrayScale {
		var f gift.Filter
		switch e.Image.GrayScaleMode {
		case 1: // average
			f = gift.ColorFunc(func(r0, g0, b0, a0 float32) (r float32, g float32, b float32, a float32) {
				y := (r0 + g0 + b0) / 3
				return y, y, y, a0
			})
		case 2: // luminance
			f = gift.ColorFunc(func(r0, g0, b0, a0 float32) (r float32, g float32, b float32, a float32) {
				y := 0.2126*r0 + 0.7152*g0 + 0.0722*b0
				return y, y, y, a0
			})
		default:
			f = gift.Grayscale()
		}
		g.Add(f)
	}

	g.Add(epubimagefilters.Pixel())

	dst := e.createImage(src, g.Bounds(src.Bounds()))
	g.Draw(dst, src)

	return epubimage.EPUBImage{
		Id:                  input.Id,
		Part:                part,
		Raw:                 dst,
		Width:               dst.Bounds().Dx(),
		Height:              dst.Bounds().Dy(),
		IsBlank:             dst.Bounds().Dx() == 1 && dst.Bounds().Dy() == 1,
		DoublePage:          isDoublePage,
		Path:                input.Path,
		Name:                input.Name,
		Format:              e.Image.Format,
		OriginalAspectRatio: float64(src.Bounds().Dy()) / float64(src.Bounds().Dx()),
		Error:               input.Error,
	}

}

type CoverTitleDataOptions struct {
	Src         image.Image
	Name        string
	Text        string
	Align       string
	PctWidth    int
	PctMargin   int
	MaxFontSize int
	BorderSize  int
}

func (e ePUBImageProcessor) cover16LevelOfGray(bounds image.Rectangle) draw.Image {
	return image.NewPaletted(bounds, color.Palette{
		color.Gray{},
		color.Gray{Y: 0x11},
		color.Gray{Y: 0x22},
		color.Gray{Y: 0x33},
		color.Gray{Y: 0x44},
		color.Gray{Y: 0x55},
		color.Gray{Y: 0x66},
		color.Gray{Y: 0x77},
		color.Gray{Y: 0x88},
		color.Gray{Y: 0x99},
		color.Gray{Y: 0xAA},
		color.Gray{Y: 0xBB},
		color.Gray{Y: 0xCC},
		color.Gray{Y: 0xDD},
		color.Gray{Y: 0xEE},
		color.Gray{Y: 0xFF},
	})
}

// CoverTitleData create a title page with the cover
func (e ePUBImageProcessor) CoverTitleData(o CoverTitleDataOptions) (epubzip.Image, error) {
	// Create a blur version of the cover
	g := gift.New(epubimagefilters.CoverTitle(o.Text, o.Align, o.PctWidth, o.PctMargin, o.MaxFontSize, o.BorderSize))
	var dst draw.Image
	if o.Name == "cover" && e.Image.GrayScale {
		dst = e.cover16LevelOfGray(o.Src.Bounds())
	} else {
		dst = e.createImage(o.Src, g.Bounds(o.Src.Bounds()))
	}
	g.Draw(dst, o.Src)

	return epubzip.CompressImage(
		"OEBPS/Images/"+o.Name+".jpeg",
		"jpeg",
		dst,
		e.Image.Quality,
	)
}
