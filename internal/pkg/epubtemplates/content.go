package epubtemplates

import (
	"github.com/beevik/etree"

	"github.com/ppkhoa/go-comic-converter/v3/internal/pkg/epubimage"
	"github.com/ppkhoa/go-comic-converter/v3/internal/pkg/utils"
	"github.com/ppkhoa/go-comic-converter/v3/pkg/epuboptions"
)

type Content struct {
	Title        string
	HasTitlePage bool
	UID          string
	Author       string
	Publisher    string
	UpdatedAt    string
	ImageOptions epuboptions.Image
	Cover        epubimage.EPUBImage
	Images       []epubimage.EPUBImage
	Current      int
	Total        int
}

type tagAttrs map[string]string

type tag struct {
	name  string
	attrs tagAttrs
	value string
}

// Get create the content file
//
//goland:noinspection HttpUrlsUsage,HttpUrlsUsage,HttpUrlsUsage,HttpUrlsUsage
func (o Content) String() string {
	doc := etree.NewDocument()
	doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)

	pkg := doc.CreateElement("package")
	pkg.CreateAttr("xmlns", "http://www.idpf.org/2007/opf")
	pkg.CreateAttr("unique-identifier", "ean")
	pkg.CreateAttr("version", "3.0")
	pkg.CreateAttr("prefix", "rendition: http://www.idpf.org/vocab/rendition/#")

	addToElement := func(elm *etree.Element, meth func() []tag) {
		for _, p := range meth() {
			meta := elm.CreateElement(p.name)
			for k, v := range p.attrs {
				meta.CreateAttr(k, v)
			}
			meta.SortAttrs()
			if p.value != "" {
				meta.CreateText(p.value)
			}
		}
	}

	metadata := pkg.CreateElement("metadata")
	metadata.CreateAttr("xmlns:dc", "http://purl.org/dc/elements/1.1/")
	metadata.CreateAttr("xmlns:opf", "http://www.idpf.org/2007/opf")
	addToElement(metadata, o.getMeta)

	manifest := pkg.CreateElement("manifest")
	addToElement(manifest, o.getManifest)

	spine := pkg.CreateElement("spine")
	if o.ImageOptions.Manga {
		spine.CreateAttr("page-progression-direction", "rtl")
	} else {
		spine.CreateAttr("page-progression-direction", "ltr")
	}

	if o.ImageOptions.View.PortraitOnly {
		addToElement(spine, o.getSpinePortrait)
	} else {
		addToElement(spine, o.getSpineAuto)
	}

	guide := pkg.CreateElement("guide")
	addToElement(guide, o.getGuide)

	doc.Indent(2)
	r, _ := doc.WriteToString()

	return r
}

// metadata part of the content
func (o Content) getMeta() []tag {
	metas := []tag{
		{"meta", tagAttrs{"property": "dcterms:modified"}, o.UpdatedAt},
		{"meta", tagAttrs{"property": "schema:accessMode"}, "visual"},
		{"meta", tagAttrs{"property": "schema:accessModeSufficient"}, "visual"},
		{"meta", tagAttrs{"property": "schema:accessibilityHazard"}, "noFlashingHazard"},
		{"meta", tagAttrs{"property": "schema:accessibilityHazard"}, "noMotionSimulationHazard"},
		{"meta", tagAttrs{"property": "schema:accessibilityHazard"}, "noSoundHazard"},
		{"meta", tagAttrs{"name": "book-type", "content": "comic"}, ""},
		{"opf:meta", tagAttrs{"name": "fixed-layout", "content": "true"}, ""},
		{"opf:meta", tagAttrs{"name": "original-resolution", "content": o.ImageOptions.View.Dimension()}, ""},
		{"dc:title", tagAttrs{}, o.Title},
		{"dc:identifier", tagAttrs{"id": "ean"}, "urn:uuid:" + o.UID},
		{"dc:language", tagAttrs{}, "en"},
		{"dc:creator", tagAttrs{}, o.Author},
		{"dc:publisher", tagAttrs{}, o.Publisher},
		{"dc:contributor", tagAttrs{}, "Go Comic Convertor"},
		{"dc:date", tagAttrs{}, o.UpdatedAt},
	}

	if o.ImageOptions.View.PortraitOnly {
		metas = append(metas, []tag{
			{"meta", tagAttrs{"property": "rendition:layout"}, "pre-paginated"},
			{"meta", tagAttrs{"property": "rendition:spread"}, "none"},
			{"meta", tagAttrs{"property": "rendition:orientation"}, "portrait"},
		}...)
	} else {
		metas = append(metas, []tag{
			{"meta", tagAttrs{"property": "rendition:layout"}, "pre-paginated"},
			{"meta", tagAttrs{"property": "rendition:spread"}, "auto"},
			{"meta", tagAttrs{"property": "rendition:orientation"}, "auto"},
		}...)
	}

	if o.ImageOptions.Manga {
		metas = append(metas, tag{"meta", tagAttrs{"name": "primary-writing-mode", "content": "horizontal-rl"}, ""})
	} else {
		metas = append(metas, tag{"meta", tagAttrs{"name": "primary-writing-mode", "content": "horizontal-lr"}, ""})
	}

	metas = append(metas, tag{"meta", tagAttrs{"name": "cover", "content": "img_cover"}, ""})

	if o.Total > 1 {
		metas = append(
			metas,
			tag{"meta", tagAttrs{"name": "calibre:series", "content": o.Title}, ""},
			tag{"meta", tagAttrs{"name": "calibre:series_index", "content": utils.IntToString(o.Current)}, ""},
		)
	}

	return metas
}

func (o Content) getManifest() []tag {
	var imageTags, pageTags, spaceTags []tag
	addTag := func(img epubimage.EPUBImage, withSpace bool) {
		imageTags = append(imageTags,
			tag{"item", tagAttrs{"id": img.ImgKey(), "href": img.ImgPath(), "media-type": img.MediaType()}, ""},
		)
		pageTags = append(pageTags,
			tag{"item", tagAttrs{"id": img.PageKey(), "href": img.PagePath(), "media-type": "application/xhtml+xml"}, ""},
		)
		if withSpace {
			spaceTags = append(spaceTags,
				tag{"item", tagAttrs{"id": img.SpaceKey(), "href": img.SpacePath(), "media-type": "application/xhtml+xml"}, ""},
			)
		}
	}

	items := []tag{
		{"item", tagAttrs{"id": "toc", "href": "toc.xhtml", "properties": "nav", "media-type": "application/xhtml+xml"}, ""},
		{"item", tagAttrs{"id": "css", "href": "Text/style.css", "media-type": "text/css"}, ""},
		{"item", tagAttrs{"id": "page_cover", "href": "Text/cover.xhtml", "media-type": "application/xhtml+xml"}, ""},
		{"item", tagAttrs{"id": "img_cover", "href": "Images/cover.jpeg", "media-type": "image/jpeg"}, ""},
	}

	if o.HasTitlePage {
		items = append(items,
			tag{"item", tagAttrs{"id": "page_title", "href": "Text/title.xhtml", "media-type": "application/xhtml+xml"}, ""},
			tag{"item", tagAttrs{"id": "img_title", "href": "Images/title.jpeg", "media-type": "image/jpeg"}, ""},
		)

		if !o.ImageOptions.View.PortraitOnly {
			items = append(items, tag{"item", tagAttrs{"id": "space_title", "href": "Text/space_title.xhtml", "media-type": "application/xhtml+xml"}, ""})
		}
	}

	lastImage := o.Images[len(o.Images)-1]
	for _, img := range o.Images {
		addTag(
			img,
			!o.ImageOptions.View.PortraitOnly &&
				(img.DoublePage ||
					(!o.ImageOptions.KeepDoublePageIfSplit && img.Part == 1) ||
					(img.Part == 0 && img == lastImage)))
	}

	items = append(items, imageTags...)
	items = append(items, pageTags...)
	items = append(items, spaceTags...)

	return items
}

// spine part of the content
func (o Content) getSpineAuto() []tag {
	isOnTheRight := !o.ImageOptions.Manga
	if o.ImageOptions.AppleBookCompatibility {
		isOnTheRight = !isOnTheRight
	}
	getSpread := func(isDoublePage bool) string {
		isOnTheRight = !isOnTheRight
		if isDoublePage {
			// Center the double page then start back to comic mode (mange/normal)
			isOnTheRight = !o.ImageOptions.Manga
			return "rendition:page-spread-center"
		}
		if isOnTheRight {
			return "rendition:page-spread-right"
		} else {
			return "rendition:page-spread-left"
		}
	}
	getSpreadBlank := func() string {
		return getSpread(false) + " layout-blank"
	}

	var spine []tag
	if o.HasTitlePage {
		if !o.ImageOptions.AppleBookCompatibility {
			spine = append(spine,
				tag{"itemref", tagAttrs{"idref": "space_title", "properties": getSpreadBlank()}, ""},
				tag{"itemref", tagAttrs{"idref": "page_title", "properties": getSpread(false)}, ""},
			)
		} else {
			spine = append(spine,
				tag{"itemref", tagAttrs{"idref": "page_title", "properties": getSpread(true)}, ""},
			)
		}
	}
	for i, img := range o.Images {
		if (img.DoublePage || img.Part == 1) && o.ImageOptions.Manga == isOnTheRight {
			spine = append(spine, tag{
				"itemref",
				tagAttrs{"idref": img.SpaceKey(), "properties": getSpreadBlank()},
				"",
			})
		}
		// register position for style adjustment
		img.Position = getSpread(img.DoublePage)
		spine = append(spine, tag{
			"itemref",
			tagAttrs{"idref": img.PageKey(), "properties": img.Position},
			"",
		})
		// save position, img is a value type
		o.Images[i] = img
	}
	if o.ImageOptions.Manga == isOnTheRight {
		spine = append(spine, tag{
			"itemref",
			tagAttrs{"idref": o.Images[len(o.Images)-1].SpaceKey(), "properties": getSpread(false)},
			"",
		})
	}

	return spine
}

func (o Content) getSpinePortrait() []tag {
	var spine []tag
	if o.HasTitlePage {
		spine = append(spine,
			tag{"itemref", tagAttrs{"idref": "page_title"}, ""},
		)
	}
	for _, img := range o.Images {
		spine = append(spine, tag{
			"itemref",
			tagAttrs{"idref": img.PageKey()},
			"",
		})
	}
	return spine
}

// getGuide Section guide of the content
func (o Content) getGuide() []tag {
	return []tag{
		{"reference", tagAttrs{"type": "cover", "title": "cover", "href": "Text/cover.xhtml"}, ""},
		{"reference", tagAttrs{"type": "text", "title": "content", "href": o.Images[0].PagePath()}, ""},
	}
}
