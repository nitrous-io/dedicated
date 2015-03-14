package amazon_test

import (
	"encoding/xml"
	"errors"
	"io/ioutil"
	"net/http"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/host/amazon"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("host/amazon", func() {
	var (
		h      host.Host
		server *ghttp.Server

		keyPairName    string
		verifyKPCalled bool
		kPCalled       bool
	)

	BeforeEach(func() {
		server = ghttp.NewServer()
		region := aws.Region{
			Name:        "mars1East",
			EC2Endpoint: server.URL(),
		}
		// we declare our own region so we can intercept the requests
		// made to our special region
		// see https://github.com/mitchellh/goamz/blob/97673455debc43e44e645466f3af7ae7a2497700/ec2/ec2_test.go#L27
		amazon.KnownRegions["test"] = region
		aws.Regions["test"] = region

		config := `
      {
        "region_id": "test",
        "name": "myinstance",
        "source_ami": "amazon-linux-2",
        "instance_type": "t1.small",
        "security_group_id": "sec1",
        "storage_size": 19
      }
    `
		var err error
		h, err = amazon.NewHost(map[string]string{
			"access_key": "amz_access_key",
			"secret_key": "amz_secret_key",
			"env":        "test",
		}, config)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		server.Close()

		if verifyKPCalled {
			Expect(kPCalled).To(BeTrue(), "DeleteKeyPair was not called")
		}
	})

	addSuccessfulKP := func() {
		resp := ec2.CreateKeyPairResp{
			RequestId:      "R1",
			KeyName:        "SomeName",
			KeyFingerprint: "xvc",
			KeyMaterial:    awsKeyMaterial,
		}
		output, err := xml.MarshalIndent(resp, "  ", "    ")
		Expect(err).To(BeNil())

		server.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/"),
				func(w http.ResponseWriter, req *http.Request) {
					Expect(req.FormValue("Action")).To(Equal("CreateKeyPair"))
					Expect(req.FormValue("KeyName")).ToNot(BeEmpty())
					keyPairName = req.FormValue("KeyName")
				},
				ghttp.RespondWith(http.StatusOK, string(output)),
			),
		)
	}

	addVerifyKP := func() {
		verifyKPCalled = true
		resp := ec2.SimpleResp{
			RequestId: "Del1",
		}
		output, err := xml.MarshalIndent(resp, "  ", "    ")
		Expect(err).To(BeNil())

		server.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/"),
				func(w http.ResponseWriter, req *http.Request) {
					kPCalled = true
					Expect(req.FormValue("Action")).To(Equal("DeleteKeyPair"))
					Expect(req.FormValue("KeyName")).To(Equal(keyPairName))
				},
				ghttp.RespondWith(http.StatusOK, output),
			),
		)
	}

	addSuccessfulImage := func() {
		resp := &ec2.ImagesResp{
			RequestId: "img1",
			Images: []ec2.Image{
				ec2.Image{
					Name: "amazon-linux-2",
				},
			},
		}
		output, err := xml.MarshalIndent(resp, "  ", "    ")
		Expect(err).To(BeNil())

		server.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/"),
				func(w http.ResponseWriter, req *http.Request) {
					Expect(req.FormValue("Action")).To(Equal("DescribeImages"))
					Expect(req.FormValue("ImageId.1")).To(Equal("amazon-linux-2"))
				},
				ghttp.RespondWith(http.StatusOK, output),
			),
		)
	}

	addSuccessfulCreate := func() {
		resp := ec2.RunInstancesResp{
			RequestId: "R1",
			Instances: []ec2.Instance{
				ec2.Instance{
					InstanceId: "my-new-instance",
				},
			},
		}
		output, err := xml.MarshalIndent(resp, "  ", "    ")
		Expect(err).To(BeNil())

		server.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/"),
				func(w http.ResponseWriter, req *http.Request) {
					Expect(req.FormValue("Action")).To(Equal("RunInstances"))
					Expect(req.FormValue("KeyName")).To(Equal(keyPairName))
					Expect(req.FormValue("ImageId")).To(Equal("amazon-linux-2"))
					Expect(req.FormValue("InstanceType")).To(Equal("t1.small"))
					Expect(req.FormValue("InstanceType")).To(Equal("t1.small"))
					Expect(req.FormValue("MinCount")).To(Equal("1"))
					Expect(req.FormValue("MaxCount")).To(Equal("1"))
					Expect(req.FormValue("ClientToken")).ToNot(BeEmpty())
					Expect(req.FormValue("SecurityGroupId.1")).To(Equal("sec1"))
					Expect(req.FormValue("SecurityGroupId.2")).To(BeEmpty())

					Expect(req.FormValue("BlockDeviceMapping.1.DeviceName")).To(Equal("/dev/sda1"))
					Expect(req.FormValue("BlockDeviceMapping.1.Ebs.VolumeType")).To(Equal("gp2"))
					Expect(req.FormValue("BlockDeviceMapping.1.Ebs.VolumeSize")).To(Equal("190"))
					Expect(req.FormValue("BlockDeviceMapping.1.Ebs.DeleteOnTermination")).To(Equal("true"))
				},
				ghttp.RespondWith(http.StatusOK, output),
			),
		)
	}

	addAssocResponse := func(ins ec2.Instance) {
		server.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/"),
				func(w http.ResponseWriter, req *http.Request) {
					Expect(req.FormValue("Action")).To(Equal("AllocateAddress"))
					Expect(req.FormValue("Domain")).To(Equal("standard"))
				},
				ghttp.RespondWith(http.StatusForbidden, ``),
			),
		)
	}

	addInstanceResponse := func(ins ec2.Instance) {
		instResp := ec2.InstancesResp{
			RequestId: "TEST1",
			Reservations: []ec2.Reservation{
				ec2.Reservation{
					Instances: []ec2.Instance{ins},
				},
			},
		}
		output, err := xml.MarshalIndent(instResp, "  ", "    ")
		Expect(err).To(BeNil())
		server.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/"),
				func(w http.ResponseWriter, req *http.Request) {
					Expect(req.FormValue("Action")).To(Equal("DescribeInstances"))
					Expect(req.FormValue("InstanceId.1")).To(Equal("my-new-instance"))
				},
				ghttp.RespondWith(http.StatusOK, string(output)),
			),
		)
	}

	FDescribe("Create()", func() {
		It("should create a temporary keypair, returning a failure", func() {
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("CreateKeyPair"))
						Expect(req.FormValue("KeyName")).ToNot(BeEmpty())
					},
					ghttp.RespondWith(http.StatusUnauthorized, ``),
				),
			)

			inf, err := h.Create(ioutil.Discard)
			Expect(err).ToNot(BeNil())
			Expect(err).To(BeAssignableToTypeOf(&ec2.Error{}))
			Expect(inf).To(BeNil())
		})

		It("should verify it couldn't decode the PEM", func() {
			resp := ec2.CreateKeyPairResp{
				RequestId:      "R1",
				KeyName:        "SomeName",
				KeyFingerprint: "xvc",
				KeyMaterial:    `Broken`,
			}
			output, err := xml.MarshalIndent(resp, "  ", "    ")
			Expect(err).To(BeNil())

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("CreateKeyPair"))
						Expect(req.FormValue("KeyName")).ToNot(BeEmpty())
					},
					ghttp.RespondWith(http.StatusOK, string(output)),
				),
			)

			inf, err := h.Create(ioutil.Discard)
			Expect(err).ToNot(BeNil())
			Expect(err).To(Equal(errors.New("failed to decode PEM, result nil")))
			Expect(inf).To(BeNil())
		})

		It("should verify it couldn't decode the PEM", func() {
			resp := ec2.CreateKeyPairResp{
				RequestId:      "R1",
				KeyName:        "SomeName",
				KeyFingerprint: "xvc",
				KeyMaterial:    corruptedAwsKeyMaterial,
			}
			output, err := xml.MarshalIndent(resp, "  ", "    ")
			Expect(err).To(BeNil())

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("CreateKeyPair"))
						Expect(req.FormValue("KeyName")).ToNot(BeEmpty())
					},
					ghttp.RespondWith(http.StatusOK, string(output)),
				),
			)

			inf, err := h.Create(ioutil.Discard)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to parse a valid RSA private key"))
			Expect(inf).To(BeNil())
		})

		It("should query for available images if the PEM is valid, returning the error and requesting the KP be deleted", func() {
			addSuccessfulKP()

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("DescribeImages"))
						Expect(req.FormValue("ImageId.1")).To(Equal("amazon-linux-2"))
					},
					ghttp.RespondWith(http.StatusForbidden, ``),
				),
			)

			addVerifyKP()

			inf, err := h.Create(ioutil.Discard)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("failed to list images"))
			Expect(inf).To(BeNil())
		})

		It("should return if the image does not exist", func() {
			addSuccessfulKP()

			resp := &ec2.ImagesResp{
				RequestId: "img1",
			}
			output, err := xml.MarshalIndent(resp, "  ", "    ")
			Expect(err).To(BeNil())

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("DescribeImages"))
						Expect(req.FormValue("ImageId.1")).To(Equal("amazon-linux-2"))
					},
					ghttp.RespondWith(http.StatusOK, output),
				),
			)

			addVerifyKP()

			inf, err := h.Create(ioutil.Discard)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring(`image "amazon-linux-2" not found`))
			Expect(inf).To(BeNil())
		})

		It("should request the instance be run, return a failure", func() {
			addSuccessfulKP()
			addSuccessfulImage()

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("RunInstances"))
						Expect(req.FormValue("KeyName")).To(Equal(keyPairName))
						Expect(req.FormValue("ImageId")).To(Equal("amazon-linux-2"))
						Expect(req.FormValue("InstanceType")).To(Equal("t1.small"))
						Expect(req.FormValue("InstanceType")).To(Equal("t1.small"))
						Expect(req.FormValue("MinCount")).To(Equal("1"))
						Expect(req.FormValue("MaxCount")).To(Equal("1"))
						Expect(req.FormValue("ClientToken")).ToNot(BeEmpty())
						Expect(req.FormValue("SecurityGroupId.1")).To(Equal("sec1"))
						Expect(req.FormValue("SecurityGroupId.2")).To(BeEmpty())

						Expect(req.FormValue("BlockDeviceMapping.1.DeviceName")).To(Equal("/dev/sda1"))
						Expect(req.FormValue("BlockDeviceMapping.1.Ebs.VolumeType")).To(Equal("gp2"))
						Expect(req.FormValue("BlockDeviceMapping.1.Ebs.VolumeSize")).To(Equal("190"))
						Expect(req.FormValue("BlockDeviceMapping.1.Ebs.DeleteOnTermination")).To(Equal("true"))
					},
					ghttp.RespondWith(http.StatusUnauthorized, ``),
				),
			)

			addVerifyKP()

			inf, err := h.Create(ioutil.Discard)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring(`failed to create new instance`))
			Expect(inf).To(BeNil())
		})

		It("should wait for a successful running state, returning an error if unmet", func() {
			addSuccessfulKP()
			addSuccessfulImage()
			addSuccessfulCreate()

			resp := ec2.Instance{
				InstanceId:      "my-new-instance",
				PublicIpAddress: "1.1.1.8",
				State: ec2.InstanceState{
					Name: "pending",
					Code: 1901,
				},
			}
			addInstanceResponse(resp)

			resp = ec2.Instance{
				InstanceId:      "my-new-instance",
				PublicIpAddress: "1.1.1.8",
				State: ec2.InstanceState{
					Name: "stopped",
					Code: 1901,
				},
			}
			addInstanceResponse(resp)

			addVerifyKP()

			inf, err := h.Create(ioutil.Discard)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring(`failed to change state unexpected state "stopped", wanted target "running"`))
			Expect(inf).To(BeNil())
		})

		It("should request an allocating of address, returning on failure", func() {
			addSuccessfulKP()
			addSuccessfulImage()
			addSuccessfulCreate()

			resp := ec2.Instance{
				InstanceId:      "my-new-instance",
				PublicIpAddress: "1.1.1.8",
				State: ec2.InstanceState{
					Name: "pending",
					Code: 1901,
				},
			}
			addInstanceResponse(resp)

			resp = ec2.Instance{
				InstanceId:      "my-new-instance",
				PublicIpAddress: "1.1.1.8",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			}
			addInstanceResponse(resp)

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("AllocateAddress"))
						Expect(req.FormValue("Domain")).To(Equal("standard"))
					},
					ghttp.RespondWith(http.StatusForbidden, ``),
				),
			)

			addVerifyKP()

			inf, err := h.Create(ioutil.Discard)
			Expect(err).ToNot(BeNil())
			Expect(err).To(BeAssignableToTypeOf(&ec2.Error{}))
			Expect(inf).To(BeNil())
		})

		It("should request an associating of address, releasing on failure", func() {
			addSuccessfulKP()
			addSuccessfulImage()
			addSuccessfulCreate()

			resp := ec2.Instance{
				InstanceId:      "my-new-instance",
				PublicIpAddress: "1.1.1.8",
				State: ec2.InstanceState{
					Name: "pending",
					Code: 1901,
				},
			}
			addInstanceResponse(resp)

			resp = ec2.Instance{
				InstanceId:      "my-new-instance",
				PublicIpAddress: "1.1.1.8",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			}
			addInstanceResponse(resp)

			addAssocResponse()

			addVerifyKP()

			inf, err := h.Create(ioutil.Discard)
			Expect(err).ToNot(BeNil())
			Expect(err).To(BeAssignableToTypeOf(&ec2.Error{}))
			Expect(inf).To(BeNil())
		})
	})

})

const corruptedAwsKeyMaterial = `-----BEGIN RSA PRIVATE KEY-----
EXAMPLEKEYKCAQEAy7WZhaDsrA1W3mRlQtvhwyORRX8gnxgDAfRt/gx42kWXsT4rXE/b5CpSgie/
vBoU7jLxx92pNHoFnByP+Dc21eyyz6CvjTmWA0JwfWiW5/akH7iO5dSrvC7dQkW2duV5QuUdE0QW
Z/aNxMniGQE6XAgfwlnXVBwrerrQo+ZWQeqiUwwMkuEbLeJFLhMCvYURpUMSC1oehm449ilx9X1F
G50TCFeOzfl8dqqCP6GzbPaIjiU19xX/azOR9V+tpUOzEL+wmXnZt3/nHPQ5xvD2OJH67km6SuPW
oPzev/D8V+x4+bHthfSjR9Y7DvQFjfBVwHXigBdtZcU2/wei8D/HYwIDAQABAoIBAGZ1kaEvnrqu
/uler7vgIn5m7lN5LKw4hJLAIW6tUT/fzvtcHK0SkbQCQXuriHmQ2MQyJX/0kn2NfjLV/ufGxbL1
mb5qwMGUnEpJaZD6QSSs3kICLwWUYUiGfc0uiSbmJoap/GTLU0W5Mfcv36PaBUNy5p53V6G7hXb2
bahyWyJNfjLe4M86yd2YK3V2CmK+X/BOsShnJ36+hjrXPPWmV3N9zEmCdJjA+K15DYmhm/tJWSD9
81oGk9TopEp7CkIfatEATyyZiVqoRq6k64iuM9JkA3OzdXzMQexXVJ1TLZVEH0E7bhlY9d8O1ozR
oQs/FiZNAx2iijCWyv0lpjE73+kCgYEA9mZtyhkHkFDpwrSM1APaL8oNAbbjwEy7Z5Mqfql+lIp1
YkriL0DbLXlvRAH+yHPRit2hHOjtUNZh4Axv+cpg09qbUI3+43eEy24B7G/Uh+GTfbjsXsOxQx/x
p9otyVwc7hsQ5TA5PZb+mvkJ5OBEKzet9XcKwONBYELGhnEPe7cCgYEA06Vgov6YHleHui9kHuws
ayav0elc5zkxjF9nfHFJRry21R1trw2Vdpn+9g481URrpzWVOEihvm+xTtmaZlSp//lkq75XDwnU
WA8gkn6O3QE3fq2yN98BURsAKdJfJ5RL1HvGQvTe10HLYYXpJnEkHv+Unl2ajLivWUt5pbBrKbUC
gYBjbO+OZk0sCcpZ29sbzjYjpIddErySIyRX5gV2uNQwAjLdp9PfN295yQ+BxMBXiIycWVQiw0bH
oMo7yykABY7Ozd5wQewBQ4AdSlWSX4nGDtsiFxWiI5sKuAAeOCbTosy1s8w8fxoJ5Tz1sdoxNeGs
Arq6Wv/G16zQuAE9zK9vvwKBgF+09VI/1wJBirsDGz9whVWfFPrTkJNvJZzYt69qezxlsjgFKshy
WBhd4xHZtmCqpBPlAymEjr/TOlbxyARmXMnIOWIAnNXMGB4KGSyl1mzSVAoQ+fqR+cJ3d0dyPl1j
jjb0Ed/NY8frlNDxAVHE8BSkdsx2f6ELEyBKJSRr9snRAoGAMrTwYneXzvTskF/S5Fyu0iOegLDa
NWUH38v/nDCgEpIXD5Hn3qAEcju1IjmbwlvtW+nY2jVhv7UGd8MjwUTNGItdb6nsYqM2asrnF3qS
VRkAKKKYeGjkpUfVTrW0YFjXkfcrR/V+QFL5OndHAKJXjW7a4ejJLncTzmZSpYzwApc=
-----END RSA PRIVATE KEY-----
`

const awsKeyMaterial = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAlm7OijRzcIsnNZWZvd2SSkjAHQ6y5gE7c/LCzihUSuJMj2vrvd/DOv5Y7O7B
EbPiFEFXJ2gR6FNr98YtmxdQcGKvFRZr60S/STDR7k16lXB/QVeM1iWLWP5LtS6v5H4dpIIfcuU3
g6hyH2RD96dpovzVZdoF9559UWYKFtxgOx3AqkeAiBFR7PEvNhYJCR0XCrYK/+VaBcXiCZSJsucv
aNzchusr8+fLgj+BEFd+s14+o4e2a82MnDfbIQZ95Hzny28NmLJuwSuyYft+dp5srfpxvH5v7+1R
d70hQJEcYaigu5d0Szla+1vwSjsnaWlsb/Cut7wxOXdb3xzlKPzdawIDAQABAoIBAHPPKDgjev+Y
UP4ihnIxoo6HhUaZa0ZAipmPuULmr0XbA5+iMKFDI1RGPQViJMZRGmoHh2Xx619U4ZilO7Ta9PUg
M7NjSH+dtDlPWT5A7W8gZQdTtDukOauwUW5RW93m7UEZX8afCebUhc7+7Ig6nMt6I3q+prtjiwxg
JsJEs9ytr9r+Euo+Z+yAd2x4QFWAfDaD9cUkSrlNaxRD7efQ6k5knQo4W5penbN464cHfxPsgAfd
xBTGpH7agvvPq4QqO1FP+ztIAg6+i78O+vj5+gMhOevrz7Mfq5GqYYM9dW8VG86TE7VfO9mR86Ys
bDk+fpD8l/anHfORErnIr/kpERECgYEA5vHsVzIaFe+Dz5GpYV7zt4hTI19rDK2jVW6c9MIEnTyv
A9mTaJhsx6n0kNR3D4+sD0ae+UJdclBexMSmqTynKHnMdLW9azUQmZcLqy+Bil58H29MurifFY8h
MEhqSIMce1ApNLEl/7rYOaSY/Kp9QFwMiFDoiF9SyUe/OZSc6akCgYEApsDOAeyYymNSaRxZy7eI
WFrdJZhwrrTxfIwyUq41SDjcB9IeDAOFaAF7BRk7KY1UTjin/pvozrNskzIPBm1278iDV/AJZktS
mWLHTc+MdPg/1vIxXBS1gJGVsG37HdNcjWX01J+yq3FmEeM3cq6XNq4TLnKk0CWiV1J/w4zRwvMC
gYAvcEZR6vK8ysRSoOMgIzcA3jnlrDX7FrfUVrJa/HeiLNLugYTsGVNO+DRKg7opMiPvx1vka6Ny
VdwBoztwzAqyH8LFvoc4PGA6jHJTKaD8YUgx1NT4DfBvp0Tz/AwI9RWmpuKv9iuOCj1cKdlbHVI5
JC9W2PfK+3mv6tL26vYIcQKBgE+s/EHrHK5Hes8CXt152kntUJFwHxc7Gc+a5UigOJFDqc6K1RSE
ci/Z1UAAaEZYYxnk9FAxirGJBa3nU0qM7zFvK2LPLI6WGudzTSueqcwR+d7oZjFLK9Y8ubtNMZl0
rlpZhvw5OhO04UJWwN2pElNsJXoiCSHzeXgj0SxMksWFAoGBAMWXGKpr0AvdJBml8EaKiistZSve
W88cGoHXrfazIHr2C8oQldmz68AowiIfgmWi+J9SxOWxbXGIPyrV8MDvGK1jVhfPcGcz85449d5z
ZdiW3Y606+pVOaOZ5c0o4hQTc+SM3tL6e4erAWO9or2j6Rawfv4GAzkpAajofhqaFWL2
-----END RSA PRIVATE KEY-----
`
