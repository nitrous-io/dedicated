package amazon_test

import (
	"encoding/xml"
	"errors"
	"net/http"
	"testing"

	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/ec2"
	"github.com/nitrous-io/dedicated/hostbuilder/host"
	"github.com/nitrous-io/dedicated/hostbuilder/host/amazon"
	"github.com/nitrous-io/dedicated/hostbuilder/state"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

func Test(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "host/amazon")
}

var _ = Describe("host/amazon", func() {
	var (
		h      host.Host
		server *ghttp.Server
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
        "region_id": "test"
      }
    `
		var err error
		h, err = amazon.NewHost(map[string]string{
			"access_key": "amz_access_key",
			"secret_key": "amz_secret_key",
		}, config)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		server.Close()
	})

	respondWithXmlStruct := func(str interface{}) {
		output, err := xml.MarshalIndent(str, "  ", "    ")
		Expect(err).To(BeNil())
		server.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/"),
				ghttp.RespondWith(http.StatusOK, string(output)),
			),
		)
	}

	Describe("NewHost", func() {
		It("should return an error if the JSON config is malformed", func() {
			config := `
        {
          "region_id": "newfoundland1
        }
      `
			var err error
			ah, err := amazon.NewHost(map[string]string{
				"access_key": "amz_access_key",
				"secret_key": "amz_secret_key",
			}, config)
			Expect(err).ToNot(BeNil())
			Expect(ah).To(BeNil())
		})

		It("should return an error if we specify an unknown region", func() {
			config := `
        {
          "region_id": "newfoundland1"
        }
      `
			var err error
			ah, err := amazon.NewHost(map[string]string{
				"access_key": "amz_access_key",
				"secret_key": "amz_secret_key",
			}, config)
			Expect(err).To(MatchError(errors.New("Invalid region: newfoundland1")))
			Expect(ah).To(BeNil())
		})
	})

	Describe("reloadInstance()", func() {
		It("should fetch the instance and return an error if it fails", func() {
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("DescribeInstances"))
						Expect(req.FormValue("InstanceId.1")).To(Equal("ABC"))
					},
					ghttp.RespondWith(http.StatusUnauthorized, ``),
				),
			)

			ah := h.(*amazon.Host)
			Expect(ah.ReloadInstance("ABC")).ToNot(BeNil())
			Expect(ah.GetInstance()).To(BeNil())
		})

		It("should return ErrInstanceNotFound if no Reservations are found", func() {
			instResp := ec2.InstancesResp{
				RequestId: "TEST1",
			}
			respondWithXmlStruct(instResp)

			ah := h.(*amazon.Host)
			Expect(ah.ReloadInstance("ABC")).To(Equal(amazon.ErrInstanceNotFound))
			Expect(ah.GetInstance()).To(BeNil())
		})

		It("should return ErrInstanceNotFound if the first Reservation lacks any Intances", func() {
			instResp := ec2.InstancesResp{
				RequestId: "TEST1",
				Reservations: []ec2.Reservation{
					ec2.Reservation{},
				},
			}
			respondWithXmlStruct(instResp)

			ah := h.(*amazon.Host)
			Expect(ah.ReloadInstance("ABC")).To(Equal(amazon.ErrInstanceNotFound))
			Expect(ah.GetInstance()).To(BeNil())
		})

		It("should assign the instance on success", func() {
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp)

			ah := h.(*amazon.Host)
			Expect(ah.ReloadInstance("ABC")).To(BeNil())
			Expect(ah.GetInstance()).ToNot(BeNil())
			Expect(ah.GetInstance().InstanceId).To(Equal("instanceT1"))
		})
	})

	Describe("Info()", func() {
		It("should return an error if the host instance is nil", func() {
			inf, err := h.Info()
			Expect(inf).To(BeNil())
			Expect(err).To(MatchError(errors.New("Invalid ec2 instance")))
		})

		It("should return information about the stored instance", func() {
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "127.89.2.49",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			})
			respondWithXmlStruct(instResp)

			ah := h.(*amazon.Host)
			Expect(ah.ReloadInstance("ABC")).To(BeNil())

			inf, err := h.Info()
			Expect(err).To(BeNil())
			Expect(inf).ToNot(BeNil())

			Expect(inf.Provider).To(Equal("amazon"))
			Expect(inf.ID).To(Equal("instanceT1"))
			Expect(inf.IPAddress).To(Equal("127.89.2.49"))
			Expect(inf.FreighterdPort).To(Equal(uint16(65333)))
			Expect(inf.Region).To(Equal("mars1East"))
			Expect(inf.State).To(Equal(state.Running))
		})

		It("should expose all the available states", func() {
			states := map[string]state.State{
				"pending":  state.Starting,
				"running":  state.Running,
				"stopping": state.Stopping,
				"stopped":  state.Stopped,
			}

			for aState, hState := range states {
				instResp := createInstanceResponse(ec2.Instance{
					InstanceId:      "instanceT1",
					PublicIpAddress: "127.89.2.49",
					State: ec2.InstanceState{
						Name: aState,
						Code: 1901,
					},
				})
				respondWithXmlStruct(instResp)

				ah := h.(*amazon.Host)
				Expect(ah.ReloadInstance("ABC")).To(BeNil())

				inf, err := h.Info()
				Expect(err).To(BeNil())
				Expect(inf).ToNot(BeNil())

				Expect(inf.State).To(Equal(hState))
			}
		})

		It("should handle an unknown EC2 state with state.Unknown", func() {
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "127.89.2.49",
				State: ec2.InstanceState{
					Name: "terminating",
					Code: 1901,
				},
			})
			respondWithXmlStruct(instResp)

			ah := h.(*amazon.Host)
			Expect(ah.ReloadInstance("ABC")).To(BeNil())

			inf, err := h.Info()
			Expect(err).To(BeNil())
			Expect(inf).ToNot(BeNil())

			Expect(inf.State).To(Equal(state.Unknown))
		})
	})
})

func createInstanceResponse(ins ec2.Instance) ec2.InstancesResp {
	return ec2.InstancesResp{
		RequestId: "TEST1",
		Reservations: []ec2.Reservation{
			ec2.Reservation{
				Instances: []ec2.Instance{ins},
			},
		},
	}
}
