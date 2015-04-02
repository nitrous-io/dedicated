package amazon

import (
	"github.com/mitchellh/goamz/ec2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type dummyImageProvider struct {
	images []ec2.Image
	calls  []fn
}

func (p *dummyImageProvider) Images(ids []string, filter *ec2.Filter) (resp *ec2.ImagesResp, err error) {
	p.calls = append(p.calls, fn{"Images", []interface{}{ids, filter}})
	images := []ec2.Image{}
	for _, i := range p.images {
		for _, id := range ids {
			if i.Id == id {
				images = append(images, i)
			}
		}
	}
	return &ec2.ImagesResp{
		Images: images,
	}, nil
}

var _ = Describe("getImage", func() {
	It("returns the first matched image", func() {
		p := &dummyImageProvider{
			images: []ec2.Image{
				{Id: "img-1234"},
				{Id: "img-2345"},
			},
		}
		img, _ := getImage(p, "img-2345")
		Expect(img.Id).To(Equal("img-2345"))
		Expect(p.calls[0].args[0].([]string)).To(Equal([]string{"img-2345"}))
	})

	It("returns error when failed to find matched image", func() {
		p := &dummyImageProvider{
			images: []ec2.Image{
				{Id: "img-1234"},
				{Id: "img-2345"},
			},
		}
		_, err := getImage(p, "img-23456")
		Expect(err).ToNot(BeNil())
	})
})
