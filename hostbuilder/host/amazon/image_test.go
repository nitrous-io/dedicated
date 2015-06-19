package amazon

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type dummyImageProvider struct {
	images []*ec2.Image
	calls  []fn
}

func (p *dummyImageProvider) DescribeImages(req *ec2.DescribeImagesInput) (*ec2.DescribeImagesOutput, error) {
	p.calls = append(p.calls, fn{"Images", []interface{}{req}})
	images := []*ec2.Image{}
	for _, i := range p.images {
		for _, id := range req.ImageIDs {
			if *i.ImageID == *id {
				images = append(images, i)
			}
		}
	}
	return &ec2.DescribeImagesOutput{
		Images: images,
	}, nil
}

var _ = Describe("getImage", func() {
	It("returns the first matched image", func() {
		p := &dummyImageProvider{
			images: []*ec2.Image{
				{ImageID: aws.String("img-1234")},
				{ImageID: aws.String("img-2345")},
			},
		}
		img, _ := getImage(p, "img-2345")
		Expect(*img.ImageID).To(Equal("img-2345"))

		req1 := p.calls[0].args[0].(*ec2.DescribeImagesInput)
		Expect(*req1.ImageIDs[0]).To(Equal("img-2345"))
	})

	It("returns error when failed to find matched image", func() {
		p := &dummyImageProvider{
			images: []*ec2.Image{
				{ImageID: aws.String("img-1234")},
				{ImageID: aws.String("img-2345")},
			},
		}
		_, err := getImage(p, "img-23456")
		Expect(err).ToNot(BeNil())
	})
})
