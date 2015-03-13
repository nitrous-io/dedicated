package amazon_test

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"io/ioutil"
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
			"env":        "test",
		}, config)
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		server.Close()
	})

	respondWithXmlStruct := func(str interface{}, instID, action string, params ...map[string]string) {
		output, err := xml.MarshalIndent(str, "  ", "    ")
		Expect(err).To(BeNil())
		server.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/"),
				func(w http.ResponseWriter, req *http.Request) {
					Expect(req.FormValue("Action")).To(Equal(action))
					Expect(req.FormValue("InstanceId.1")).To(Equal(instID))
					if len(params) > 0 {
						for k, v := range params[0] {
							Expect(req.FormValue(k)).To(Equal(v))
						}
					}
				},
				ghttp.RespondWith(http.StatusOK, string(output)),
			),
		)
	}

	respondWithEmptyInstances := func(instID string) {
		instResp := ec2.InstancesResp{
			RequestId: "instanceT1",
		}
		output, err := xml.MarshalIndent(instResp, "  ", "    ")
		Expect(err).To(BeNil())
		server.AppendHandlers(
			ghttp.CombineHandlers(
				ghttp.VerifyRequest("GET", "/"),
				func(w http.ResponseWriter, req *http.Request) {
					Expect(req.FormValue("Action")).To(Equal("DescribeInstances"))
					Expect(req.FormValue("InstanceId.1")).To(Equal(instID))
				},
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
			Expect(err).To(BeAssignableToTypeOf(&json.SyntaxError{}))
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
						Expect(req.FormValue("InstanceId.1")).To(Equal("instanceT1"))
					},
					ghttp.RespondWith(http.StatusUnauthorized, ``),
				),
			)

			ah := h.(*amazon.Host)
			Expect(ah.ReloadInstance("instanceT1")).ToNot(BeNil())
			Expect(ah.GetInstance()).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})

		It("should return ErrInstanceNotFound if no Reservations are found", func() {
			instResp := ec2.InstancesResp{
				RequestId: "TEST1",
			}
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			ah := h.(*amazon.Host)
			Expect(ah.ReloadInstance("instanceT1")).To(Equal(amazon.ErrInstanceNotFound))
			Expect(ah.GetInstance()).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})

		It("should return ErrInstanceNotFound if the first Reservation lacks any Intances", func() {
			instResp := ec2.InstancesResp{
				RequestId: "TEST1",
				Reservations: []ec2.Reservation{
					ec2.Reservation{},
				},
			}
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			ah := h.(*amazon.Host)
			Expect(ah.ReloadInstance("instanceT1")).To(Equal(amazon.ErrInstanceNotFound))
			Expect(ah.GetInstance()).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})

		It("should assign the instance on success", func() {
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			ah := h.(*amazon.Host)
			Expect(ah.ReloadInstance("instanceT1")).To(BeNil())
			Expect(ah.GetInstance()).ToNot(BeNil())
			Expect(ah.GetInstance().InstanceId).To(Equal("instanceT1"))
			Expect(server.ReceivedRequests()).To(HaveLen(1))
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
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			ah := h.(*amazon.Host)
			Expect(ah.ReloadInstance("instanceT1")).To(BeNil())

			inf, err := h.Info()
			Expect(err).To(BeNil())
			Expect(inf).ToNot(BeNil())

			Expect(inf.Provider).To(Equal("amazon"))
			Expect(inf.ID).To(Equal("instanceT1"))
			Expect(inf.IPAddress).To(Equal("127.89.2.49"))
			Expect(inf.FreighterdPort).To(Equal(uint16(65333)))
			Expect(inf.Region).To(Equal("mars1East"))
			Expect(inf.State).To(Equal(state.Running))
			Expect(server.ReceivedRequests()).To(HaveLen(1))
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
				respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

				ah := h.(*amazon.Host)
				Expect(ah.ReloadInstance("instanceT1")).To(BeNil())

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
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			ah := h.(*amazon.Host)
			Expect(ah.ReloadInstance("instanceT1")).To(BeNil())

			inf, err := h.Info()
			Expect(err).To(BeNil())
			Expect(inf).ToNot(BeNil())

			Expect(inf.State).To(Equal(state.Unknown))
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})
	})

	Describe("Update()", func() {
		BeforeEach(func() {
			ah := h.(*amazon.Host)
			ah.Config = &amazon.HostConfig{
				ID:   "instanceT1",
				Name: "MyNewName",
			}
		})

		It("should pass through the error if it was unable to reload the instance", func() {
			respondWithEmptyInstances("instanceT1")

			inf, err := h.Update(ioutil.Discard)
			Expect(err).To(Equal(amazon.ErrInstanceNotFound))
			Expect(inf).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})

		It("should associate tags", func() {
			// respond with the struct needed for reload
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			// verify the CreateTags request
			resp := ec2.SimpleResp{
				RequestId: "S1",
			}
			output, err := xml.MarshalIndent(resp, "  ", "    ")
			Expect(err).To(BeNil())
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("CreateTags"))
						Expect(req.FormValue("ResourceId.1")).To(Equal("instanceT1"))
						Expect(req.FormValue("Tag.1.Key")).To(Equal("Name"))
						Expect(req.FormValue("Tag.1.Value")).To(Equal("MyNewName"))
						Expect(req.FormValue("Tag.2.Key")).To(Equal("dedicated"))
						Expect(req.FormValue("Tag.2.Value")).To(Equal("test"))
					},
					ghttp.RespondWith(http.StatusOK, string(output)),
				),
			)

			inf, err := h.Update(ioutil.Discard)
			Expect(err).To(BeNil())
			inf2, err := h.Info()
			Expect(err).To(BeNil())
			Expect(inf).To(Equal(inf2))
			Expect(server.ReceivedRequests()).To(HaveLen(2))
		})

		It("should swallow the endpoint failing", func() {
			// respond with the struct needed for reload
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("CreateTags"))
						Expect(req.FormValue("ResourceId.1")).To(Equal("instanceT1"))
					},
					ghttp.RespondWith(http.StatusUnauthorized, ``),
				),
			)

			inf, err := h.Update(ioutil.Discard)
			Expect(err).To(BeNil())
			Expect(inf).ToNot(BeNil())
			inf2, err := h.Info()
			Expect(err).To(BeNil())
			Expect(inf).To(Equal(inf2))
			Expect(server.ReceivedRequests()).To(HaveLen(2))
		})
	})

	Describe("Stop()", func() {
		BeforeEach(func() {
			ah := h.(*amazon.Host)
			ah.Config = &amazon.HostConfig{
				ID:   "instanceT1",
				Name: "MyNewName",
			}
		})

		It("should pass through the error if it was unable to reload the instance", func() {
			respondWithEmptyInstances("instanceT1")

			inf, err := h.Stop()
			Expect(err).To(Equal(amazon.ErrInstanceNotFound))
			Expect(inf).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})

		It("should request the instance be stopped, returning a failure", func() {
			// respond with the struct needed for reload
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			resp := ec2.StopInstanceResp{
				RequestId: "S1",
			}
			output, err := xml.MarshalIndent(resp, "  ", "    ")
			Expect(err).To(BeNil())

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("StopInstances"))
						Expect(req.FormValue("InstanceId.1")).To(Equal("instanceT1"))
					},
					ghttp.RespondWith(http.StatusUnauthorized, string(output)),
				),
			)

			inf, err := h.Stop()
			Expect(err).ToNot(BeNil())
			Expect(err).To(BeAssignableToTypeOf(&ec2.Error{}))
			Expect(inf).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(2))
		})

		It("should return an error if the host entered into an unexpected state", func() {
			// respond with the struct needed for reload
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			resp := ec2.StopInstanceResp{
				RequestId: "S1",
			}
			respondWithXmlStruct(resp, "instanceT1", "StopInstances")

			// respond with bad state
			termResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
				State: ec2.InstanceState{
					Name: "terminated",
					Code: 1901,
				},
			})
			respondWithXmlStruct(termResp, "instanceT1", "DescribeInstances")

			inf, err := h.Stop()
			Expect(err).To(MatchError(errors.New(`unexpected state "terminated", wanted target "stopped"`)))
			Expect(inf).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(3))
		})

		It("should return the instance when host enters into a stopped state", func() {
			// respond with the struct needed for reload
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			resp := ec2.StopInstanceResp{
				RequestId: "S1",
			}
			respondWithXmlStruct(resp, "instanceT1", "StopInstances")

			// respond with an intermediate state
			stoppingResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
				State: ec2.InstanceState{
					Name: "stopping",
					Code: 1901,
				},
			})
			respondWithXmlStruct(stoppingResp, "instanceT1", "DescribeInstances")

			// respond with the final state state
			stoppedResp := createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "192.168.1.51",
				State: ec2.InstanceState{
					Name: "stopped",
					Code: 1901,
				},
			})
			respondWithXmlStruct(stoppedResp, "instanceT1", "DescribeInstances")

			inf, err := h.Stop()
			Expect(err).To(BeNil())
			Expect(inf).ToNot(BeNil())
			Expect(inf.State).To(Equal(state.Stopped))
			Expect(inf.IPAddress).To(Equal("192.168.1.51"))
			Expect(inf.ID).To(Equal("instanceT1"))
			Expect(server.ReceivedRequests()).To(HaveLen(4))
		})

		It("should wait for the operation to transition from pending states", func() {
			// respond with the struct needed for reload
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			resp := ec2.StopInstanceResp{
				RequestId: "S1",
			}
			respondWithXmlStruct(resp, "instanceT1", "StopInstances")

			// respond with transitions
			stoppedResp := createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "192.168.1.51",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			})
			respondWithXmlStruct(stoppedResp, "instanceT1", "DescribeInstances")

			stoppedResp = createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "192.168.1.51",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			})
			respondWithXmlStruct(stoppedResp, "instanceT1", "DescribeInstances")

			stoppedResp = createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "192.168.1.51",
				State: ec2.InstanceState{
					Name: "pending",
					Code: 1901,
				},
			})
			respondWithXmlStruct(stoppedResp, "instanceT1", "DescribeInstances")

			stoppedResp = createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "192.168.1.51",
				State: ec2.InstanceState{
					Name: "stopping",
					Code: 1901,
				},
			})
			respondWithXmlStruct(stoppedResp, "instanceT1", "DescribeInstances")

			stoppedResp = createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "192.168.1.51",
				State: ec2.InstanceState{
					Name: "stopped",
					Code: 1901,
				},
			})
			respondWithXmlStruct(stoppedResp, "instanceT1", "DescribeInstances")

			inf, err := h.Stop()
			Expect(err).To(BeNil())
			Expect(inf).ToNot(BeNil())
			Expect(inf.State).To(Equal(state.Stopped))
			Expect(inf.IPAddress).To(Equal("192.168.1.51"))
			Expect(inf.ID).To(Equal("instanceT1"))
			Expect(server.ReceivedRequests()).To(HaveLen(7))
		})
	})

	Describe("Start()", func() {
		BeforeEach(func() {
			ah := h.(*amazon.Host)
			ah.Config = &amazon.HostConfig{
				ID:        "instanceT1",
				Name:      "MyNewName",
				IPAddress: "10.1.1.250",
			}
		})

		It("should pass through the error if it was unable to reload the instance", func() {
			respondWithEmptyInstances("instanceT1")

			inf, err := h.Start()
			Expect(err).To(Equal(amazon.ErrInstanceNotFound))
			Expect(inf).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})

		It("should request the instance be stopped, returning a failure", func() {
			// respond with the struct needed for reload
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("StartInstances"))
						Expect(req.FormValue("InstanceId.1")).To(Equal("instanceT1"))
					},
					ghttp.RespondWith(http.StatusUnauthorized, ``),
				),
			)

			inf, err := h.Start()
			Expect(err).ToNot(BeNil())
			Expect(err).To(BeAssignableToTypeOf(&ec2.Error{}))
			Expect(inf).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(2))
		})

		It("should return an error if the host entered into an unexpected state", func() {
			// respond with the struct needed for reload
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			resp := ec2.StartInstanceResp{
				RequestId: "S1",
			}
			output, err := xml.MarshalIndent(resp, "  ", "    ")
			Expect(err).To(BeNil())
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("StartInstances"))
						Expect(req.FormValue("InstanceId.1")).To(Equal("instanceT1"))
					},
					ghttp.RespondWith(http.StatusOK, string(output)),
				),
			)

			// respond with the struct needed for reload
			termResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
				State: ec2.InstanceState{
					Name: "terminated",
					Code: 1901,
				},
			})
			respondWithXmlStruct(termResp, "instanceT1", "DescribeInstances")

			inf, err := h.Start()
			Expect(err).To(MatchError(errors.New(`unexpected state "terminated", wanted target "running"`)))
			Expect(inf).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(3))
		})

		It("should return an error if it failed to associate addresses", func() {
			// respond with the struct needed for reload
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			resp := ec2.StartInstanceResp{
				RequestId: "S1",
			}
			respondWithXmlStruct(resp, "instanceT1", "StartInstances")

			// respond with the struct needed for reload
			runningResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("AssociateAddress"))
						Expect(req.FormValue("InstanceId")).To(Equal("instanceT1"))
						Expect(req.FormValue("PublicIp")).To(Equal("10.1.1.250"))
						Expect(req.FormValue("AllowReassociation")).To(BeEmpty())
					},
					ghttp.RespondWith(http.StatusForbidden, ``),
				),
			)

			inf, err := h.Start()
			Expect(err).ToNot(BeNil())
			Expect(err).To(BeAssignableToTypeOf(&ec2.Error{}))
			Expect(inf).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(4))
		})

		It("should return an error if the final reload failed", func() {
			// respond with the struct needed for reload
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			resp := ec2.StartInstanceResp{
				RequestId: "S1",
			}
			respondWithXmlStruct(resp, "instanceT1", "StartInstances")

			// respond with the struct needed for reload
			runningResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			assocResp := ec2.AssociateAddressResp{
				RequestId: "S2",
				Return:    true,
			}
			output, err := xml.MarshalIndent(assocResp, "  ", "    ")
			Expect(err).To(BeNil())
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("AssociateAddress"))
						Expect(req.FormValue("InstanceId")).To(Equal("instanceT1"))
						Expect(req.FormValue("PublicIp")).To(Equal("10.1.1.250"))
						Expect(req.FormValue("AllowReassociation")).To(BeEmpty())
					},
					ghttp.RespondWith(http.StatusOK, output),
				),
			)

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("DescribeInstances"))
						Expect(req.FormValue("InstanceId.1")).To(Equal("instanceT1"))
					},
					ghttp.RespondWith(http.StatusForbidden, ``),
				),
			)

			inf, err := h.Start()
			Expect(err).ToNot(BeNil())
			Expect(err).To(BeAssignableToTypeOf(&ec2.Error{}))
			Expect(inf).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(5))
		})

		It("should return the reloaded instance on a successful start", func() {
			// respond with the struct needed for reload
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			resp := ec2.StartInstanceResp{
				RequestId: "S1",
			}
			respondWithXmlStruct(resp, "instanceT1", "StartInstances")

			// respond with the struct needed for reload
			runningResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			assocResp := ec2.AssociateAddressResp{
				RequestId: "S2",
				Return:    true,
			}
			output, err := xml.MarshalIndent(assocResp, "  ", "    ")
			Expect(err).To(BeNil())
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("AssociateAddress"))
						Expect(req.FormValue("InstanceId")).To(Equal("instanceT1"))
						Expect(req.FormValue("PublicIp")).To(Equal("10.1.1.250"))
						Expect(req.FormValue("AllowReassociation")).To(BeEmpty())
					},
					ghttp.RespondWith(http.StatusOK, output),
				),
			)

			runningResp = createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "1.1.1.8",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			inf, err := h.Start()
			Expect(err).To(BeNil())
			Expect(inf).ToNot(BeNil())
			Expect(inf.ID).To(Equal("instanceT1"))
			Expect(inf.IPAddress).To(Equal("1.1.1.8"))
			Expect(server.ReceivedRequests()).To(HaveLen(5))
		})

		It("should wait for the operation to transition from pending states", func() {
			// respond with the struct needed for reload
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			resp := ec2.StartInstanceResp{
				RequestId: "S1",
			}
			respondWithXmlStruct(resp, "instanceT1", "StartInstances")

			// respond with transitions
			runningResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
				State: ec2.InstanceState{
					Name: "stopped",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			runningResp = createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
				State: ec2.InstanceState{
					Name: "pending",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			runningResp = createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			assocResp := ec2.AssociateAddressResp{
				RequestId: "S2",
				Return:    true,
			}
			output, err := xml.MarshalIndent(assocResp, "  ", "    ")
			Expect(err).To(BeNil())
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("AssociateAddress"))
						Expect(req.FormValue("InstanceId")).To(Equal("instanceT1"))
						Expect(req.FormValue("PublicIp")).To(Equal("10.1.1.250"))
						Expect(req.FormValue("AllowReassociation")).To(BeEmpty())
					},
					ghttp.RespondWith(http.StatusOK, output),
				),
			)

			runningResp = createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "1.1.1.8",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			inf, err := h.Start()
			Expect(err).To(BeNil())
			Expect(inf).ToNot(BeNil())
			Expect(inf.ID).To(Equal("instanceT1"))
			Expect(inf.IPAddress).To(Equal("1.1.1.8"))
			Expect(server.ReceivedRequests()).To(HaveLen(7))
		})
	})

	Describe("Restart()", func() {
		BeforeEach(func() {
			ah := h.(*amazon.Host)
			ah.Config = &amazon.HostConfig{
				ID:        "instanceT1",
				Name:      "MyNewName",
				IPAddress: "10.1.1.250",
			}
		})

		It("should return an error if it fails get info about the instance", func() {
			respondWithEmptyInstances("instanceT1")

			inf, err := h.Restart()
			Expect(err).To(Equal(amazon.ErrInstanceNotFound))
			Expect(inf).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})

		It("should request the instance be stopped, waiting for it to be stopped", func() {
			// respond with the struct needed for reload
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			resp := ec2.StopInstanceResp{
				RequestId: "S1",
			}
			respondWithXmlStruct(resp, "instanceT1", "StopInstances")

			// respond with transitions
			stoppedResp := createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "192.168.1.51",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			})
			respondWithXmlStruct(stoppedResp, "instanceT1", "DescribeInstances")

			termResp := createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "192.168.1.51",
				State: ec2.InstanceState{
					Name: "terminated",
					Code: 1901,
				},
			})
			respondWithXmlStruct(termResp, "instanceT1", "DescribeInstances")

			inf, err := h.Restart()
			Expect(err).To(MatchError(errors.New(`unexpected state "terminated", wanted target "stopped"`)))
			Expect(inf).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(4))
		})

		It("should attempt to start the container after successfully stopping, returning failure", func() {
			// respond with the struct needed for reload
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			resp := ec2.StopInstanceResp{
				RequestId: "S1",
			}
			respondWithXmlStruct(resp, "instanceT1", "StopInstances")

			// respond with transitions
			runningResp := createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "192.168.1.51",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			stoppedResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
				State: ec2.InstanceState{
					Name: "stopped",
					Code: 1901,
				},
			})
			respondWithXmlStruct(stoppedResp, "instanceT1", "DescribeInstances")

			// Start() beings

			// respond with the struct needed for reload
			instResp = createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("StartInstances"))
						Expect(req.FormValue("InstanceId.1")).To(Equal("instanceT1"))
					},
					ghttp.RespondWith(http.StatusUnauthorized, ``),
				),
			)

			inf, err := h.Restart()
			Expect(err).ToNot(BeNil())
			Expect(err).To(BeAssignableToTypeOf(&ec2.Error{}))
			Expect(inf).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(6))
		})

		It("should attempt to start the container after successfully stopping, associating the IP address", func() {
			// respond with the struct needed for reload
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			stopResp := ec2.StopInstanceResp{
				RequestId: "S1",
			}
			respondWithXmlStruct(stopResp, "instanceT1", "StopInstances")

			// respond with transitions
			runningResp := createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "192.168.1.51",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			stoppedResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
				State: ec2.InstanceState{
					Name: "stopped",
					Code: 1901,
				},
			})
			respondWithXmlStruct(stoppedResp, "instanceT1", "DescribeInstances")

			// Start() beings

			// respond with the struct needed for reload
			instResp = createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			startResp := ec2.StartInstanceResp{
				RequestId: "S1",
			}
			respondWithXmlStruct(startResp, "instanceT1", "StartInstances")

			runningResp = createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
				State: ec2.InstanceState{
					Name: "pending",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			runningResp = createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			assocResp := ec2.AssociateAddressResp{
				RequestId: "S2",
				Return:    true,
			}
			output, err := xml.MarshalIndent(assocResp, "  ", "    ")
			Expect(err).To(BeNil())
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("AssociateAddress"))
						Expect(req.FormValue("InstanceId")).To(Equal("instanceT1"))
						Expect(req.FormValue("PublicIp")).To(Equal("10.1.1.250"))
						Expect(req.FormValue("AllowReassociation")).To(BeEmpty())
					},
					ghttp.RespondWith(http.StatusOK, output),
				),
			)

			runningResp = createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "1.1.1.8",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			inf, err := h.Restart()
			Expect(err).To(BeNil())
			Expect(inf).ToNot(BeNil())
			Expect(inf.ID).To(Equal("instanceT1"))
			Expect(inf.IPAddress).To(Equal("1.1.1.8"))
			Expect(server.ReceivedRequests()).To(HaveLen(10))
		})
	})

	Describe("Destroy()", func() {
		BeforeEach(func() {
			ah := h.(*amazon.Host)
			ah.Config = &amazon.HostConfig{
				ID:        "instanceT1",
				Name:      "MyNewName",
				IPAddress: "10.1.1.250",
			}
		})

		It("should not return an error if the instance already does nor exist", func() {
			respondWithEmptyInstances("instanceT1")

			err := h.Destroy()
			Expect(err).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})

		It("should forward an error if reload fails", func() {
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("DescribeInstances"))
						Expect(req.FormValue("InstanceId.1")).To(Equal("instanceT1"))
					},
					ghttp.RespondWith(http.StatusUnauthorized, ``),
				),
			)

			err := h.Destroy()
			Expect(err).ToNot(BeNil())
			Expect(err).To(BeAssignableToTypeOf(&ec2.Error{}))
			Expect(server.ReceivedRequests()).To(HaveLen(1))
		})

		It("should request the IP Address of the instance be released, feeding back failure", func() {
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("ReleaseAddress"))
						Expect(req.FormValue("PublicIp")).To(Equal("10.1.1.250"))
					},
					ghttp.RespondWith(http.StatusUnauthorized, ``),
				),
			)

			err := h.Destroy()
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("amazon: failed to release ElasticIP 10.1.1.250 for instanceT1"))
			Expect(server.ReceivedRequests()).To(HaveLen(2))
		})

		It("should request the instance be terminated, feeding back failure", func() {
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			resp := ec2.SimpleResp{
				RequestId: "S1",
			}
			output, err := xml.MarshalIndent(resp, "  ", "    ")
			Expect(err).To(BeNil())
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("ReleaseAddress"))
						Expect(req.FormValue("PublicIp")).To(Equal("10.1.1.250"))
					},
					ghttp.RespondWith(http.StatusOK, string(output)),
				),
			)

			Expect(err).To(BeNil())
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("TerminateInstances"))
						Expect(req.FormValue("InstanceId.1")).To(Equal("instanceT1"))
					},
					ghttp.RespondWith(http.StatusUnauthorized, ``),
				),
			)

			err = h.Destroy()
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("amazon: failed to terminate instance instanceT1"))
			Expect(server.ReceivedRequests()).To(HaveLen(3))
		})

		It("should wait for the state, returning a failure if state was unexpected", func() {
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			resp := ec2.SimpleResp{
				RequestId: "S1",
			}
			output, err := xml.MarshalIndent(resp, "  ", "    ")
			Expect(err).To(BeNil())
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("ReleaseAddress"))
						Expect(req.FormValue("PublicIp")).To(Equal("10.1.1.250"))
					},
					ghttp.RespondWith(http.StatusOK, string(output)),
				),
			)

			termResp := ec2.TerminateInstancesResp{
				RequestId: "S1",
			}
			output, err = xml.MarshalIndent(termResp, "  ", "    ")
			Expect(err).To(BeNil())
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("TerminateInstances"))
						Expect(req.FormValue("InstanceId.1")).To(Equal("instanceT1"))
					},
					ghttp.RespondWith(http.StatusOK, string(output)),
				),
			)

			runningResp := createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "1.1.1.8",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			runningResp = createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "1.1.1.8",
				State: ec2.InstanceState{
					Name: "swapping",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			err = h.Destroy()
			Expect(err).To(Equal(errors.New(`unexpected state "swapping", wanted target "terminated"`)))
			Expect(server.ReceivedRequests()).To(HaveLen(5))
		})

		It("should return nil when the instance terminates", func() {
			instResp := createInstanceResponse(ec2.Instance{
				InstanceId: "instanceT1",
			})
			respondWithXmlStruct(instResp, "instanceT1", "DescribeInstances")

			resp := ec2.SimpleResp{
				RequestId: "S1",
			}
			output, err := xml.MarshalIndent(resp, "  ", "    ")
			Expect(err).To(BeNil())
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("ReleaseAddress"))
						Expect(req.FormValue("PublicIp")).To(Equal("10.1.1.250"))
					},
					ghttp.RespondWith(http.StatusOK, string(output)),
				),
			)

			termResp := ec2.TerminateInstancesResp{
				RequestId: "S1",
			}
			output, err = xml.MarshalIndent(termResp, "  ", "    ")
			Expect(err).To(BeNil())
			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("TerminateInstances"))
						Expect(req.FormValue("InstanceId.1")).To(Equal("instanceT1"))
					},
					ghttp.RespondWith(http.StatusOK, string(output)),
				),
			)

			runningResp := createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "1.1.1.8",
				State: ec2.InstanceState{
					Name: "running",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			runningResp = createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "1.1.1.8",
				State: ec2.InstanceState{
					Name: "shutting-down",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			runningResp = createInstanceResponse(ec2.Instance{
				InstanceId:      "instanceT1",
				PublicIpAddress: "1.1.1.8",
				State: ec2.InstanceState{
					Name: "terminated",
					Code: 1901,
				},
			})
			respondWithXmlStruct(runningResp, "instanceT1", "DescribeInstances")

			err = h.Destroy()
			Expect(err).To(BeNil())
			Expect(server.ReceivedRequests()).To(HaveLen(6))

			ah := h.(*amazon.Host)
			inst := ah.GetInstance()
			Expect(inst.State.Name).To(Equal("terminated"))
		})
	})

	Describe("List()", func() {
		var scndSvr *ghttp.Server
		var allServers []*ghttp.Server
		var originalRegions map[string]aws.Region

		BeforeEach(func() {
			scndSvr = ghttp.NewServer()
			scndRegion := aws.Region{
				Name:        "pluto1North",
				EC2Endpoint: scndSvr.URL(),
			}
			region := aws.Region{
				Name:        "mars1East",
				EC2Endpoint: server.URL(),
			}

			originalRegions = amazon.KnownRegions
			amazon.KnownRegions = map[string]aws.Region{
				"test":  region,
				"otest": scndRegion,
			}
			allServers = []*ghttp.Server{
				server,
				scndSvr,
			}
		})

		AfterEach(func() {
			if originalRegions != nil {
				amazon.KnownRegions = originalRegions
			}
		})

		It("should filter by the environment", func() {
			instResp := ec2.InstancesResp{
				RequestId: "instanceT1",
			}
			output, err := xml.MarshalIndent(instResp, "  ", "    ")
			Expect(err).To(BeNil())
			for _, svr := range allServers {
				svr.AppendHandlers(
					ghttp.CombineHandlers(
						ghttp.VerifyRequest("GET", "/"),
						func(w http.ResponseWriter, req *http.Request) {
							Expect(req.FormValue("Action")).To(Equal("DescribeInstances"))
							Expect(req.FormValue("Filter.1.Name")).To(Equal("tag:dedicated"))
							Expect(req.FormValue("Filter.1.Value.1")).To(Equal("test"))
						},
						ghttp.RespondWith(http.StatusOK, string(output)),
					),
				)
			}

			infos, err := h.List()
			Expect(err).To(BeNil())
			Expect(infos).To(BeEmpty())
			for _, svr := range allServers {
				Expect(svr.ReceivedRequests()).To(HaveLen(1))
			}
		})

		It("should collate all instances from the regions", func() {
			instMars := []ec2.Instance{
				ec2.Instance{
					InstanceId:      "instance1",
					PublicIpAddress: "127.89.2.49",
					State: ec2.InstanceState{
						Name: "running",
						Code: 1901,
					},
				},
				ec2.Instance{
					InstanceId:      "instance2",
					PublicIpAddress: "10.1.2.5",
					State: ec2.InstanceState{
						Name: "stopping",
						Code: 1901,
					},
				},
			}

			instPluto := []ec2.Instance{
				ec2.Instance{
					InstanceId:      "instance3",
					PublicIpAddress: "19.1.1.8",
					State: ec2.InstanceState{
						Name: "stopped",
						Code: 1901,
					},
				},
			}

			instPluto2 := []ec2.Instance{
				ec2.Instance{
					InstanceId:      "instance4",
					PublicIpAddress: "19.2.1.8",
					State: ec2.InstanceState{
						Name: "pending",
						Code: 1901,
					},
				},
			}

			marsResp := ec2.InstancesResp{
				RequestId: "TEST1",
				Reservations: []ec2.Reservation{
					ec2.Reservation{
						Instances: instMars,
					},
				},
			}

			plutoResp := ec2.InstancesResp{
				RequestId: "TEST2",
				Reservations: []ec2.Reservation{
					ec2.Reservation{
						Instances: instPluto,
					},
					ec2.Reservation{
						Instances: instPluto2,
					},
				},
			}

			marsOutput, err := xml.MarshalIndent(marsResp, "  ", "    ")
			Expect(err).To(BeNil())
			plutoOutput, err := xml.MarshalIndent(plutoResp, "  ", "    ")
			Expect(err).To(BeNil())

			server.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("DescribeInstances"))
						Expect(req.FormValue("Filter.1.Name")).To(Equal("tag:dedicated"))
						Expect(req.FormValue("Filter.1.Value.1")).To(Equal("test"))
					},
					ghttp.RespondWith(http.StatusOK, string(marsOutput)),
				),
			)

			scndSvr.AppendHandlers(
				ghttp.CombineHandlers(
					ghttp.VerifyRequest("GET", "/"),
					func(w http.ResponseWriter, req *http.Request) {
						Expect(req.FormValue("Action")).To(Equal("DescribeInstances"))
						Expect(req.FormValue("Filter.1.Name")).To(Equal("tag:dedicated"))
						Expect(req.FormValue("Filter.1.Value.1")).To(Equal("test"))
					},
					ghttp.RespondWith(http.StatusOK, string(plutoOutput)),
				),
			)

			infos, err := h.List()
			Expect(err).To(BeNil())
			Expect(infos).To(HaveLen(4))
			for _, svr := range allServers {
				Expect(svr.ReceivedRequests()).To(HaveLen(1))
			}

			var i1, i2, i3, i4 *host.Info
			for _, info := range infos {
				switch info.ID {
				case "instance1":
					i1 = info
				case "instance2":
					i2 = info
				case "instance3":
					i3 = info
				case "instance4":
					i4 = info
				default:
					Fail("Unexpected instance")
				}
			}

			Expect(i1.IPAddress).To(Equal("127.89.2.49"))
			Expect(i1.State).To(Equal(state.Running))
			Expect(i1.Provider).To(Equal("amazon"))
			Expect(i1.FreighterdPort).To(Equal(uint16(65333)))
			Expect(i1.Region).To(Equal("mars1East"))

			Expect(i2.IPAddress).To(Equal("10.1.2.5"))
			Expect(i2.State).To(Equal(state.Stopping))
			Expect(i2.Provider).To(Equal("amazon"))
			Expect(i2.FreighterdPort).To(Equal(uint16(65333)))
			Expect(i2.Region).To(Equal("mars1East"))

			Expect(i3.IPAddress).To(Equal("19.1.1.8"))
			Expect(i3.State).To(Equal(state.Stopped))
			Expect(i3.Provider).To(Equal("amazon"))
			Expect(i3.FreighterdPort).To(Equal(uint16(65333)))
			Expect(i3.Region).To(Equal("pluto1North"))

			Expect(i4.IPAddress).To(Equal("19.2.1.8"))
			Expect(i4.State).To(Equal(state.Starting))
			Expect(i4.Provider).To(Equal("amazon"))
			Expect(i4.FreighterdPort).To(Equal(uint16(65333)))
			Expect(i4.Region).To(Equal("pluto1North"))
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
