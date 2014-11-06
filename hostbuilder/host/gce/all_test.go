package gce

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestGCE(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "host/gce")
}

var (
	getInstanceResponse = `
	{
		"kind": "compute#instance",
		"id": "5185169717923901487",
		"creationTimestamp": "2014-11-06T00:44:16.826-08:00",
		"zone": "https://www.googleapis.com/compute/v1/projects/ivory-haven-742/zones/asia-east1-c",
		"status": "RUNNING",
		"name": "supersonic-harrenhal-host-55",
		"tags": {
			"fingerprint": "42WmSpB8rSM="
		},
		"machineType": "https://www.googleapis.com/compute/v1/projects/ivory-haven-742/zones/asia-east1-c/machineTypes/g1-small",
		"canIpForward": false,
		"networkInterfaces": [
		{
			"network": "https://www.googleapis.com/compute/v1/projects/ivory-haven-742/global/networks/default",
			"networkIP": "10.240.19.30",
			"name": "nic0",
			"accessConfigs": [
			{
				"kind": "compute#accessConfig",
				"type": "ONE_TO_ONE_NAT",
				"name": "AccessConfig",
				"natIP": "130.211.240.56"
			}
			]
		}
		],
		"disks": [
		{
			"kind": "compute#attachedDisk",
			"index": 0,
			"type": "PERSISTENT",
			"mode": "READ_WRITE",
			"source": "https://www.googleapis.com/compute/v1/projects/ivory-haven-742/zones/asia-east1-c/disks/supersonic-harrenhal-host-55",
			"deviceName": "persistent-disk-0",
			"boot": true,
			"autoDelete": true,
			"interface": "SCSI"
		}
		],
		"metadata": {
			"kind": "compute#metadata",
			"fingerprint": "bdlZbvP5MVM=",
			"items": [
			{
				"key": "sshKeys",
				"value": "root:ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCaO1Qux4UgFHO7TZEShm1cY9itQuze+iRD0v3ByLZv82Hw+xpQjZFjejZMuK2bljgJJ0+x0ScmkltnJf3O+vhGXIf3EoHxY8zOVRwtvTNieH5kJAdMHCrxUO/92ifmfHme85+36zJb1YMIHjlR1L+AqwzdG6+b1tJ80gYdpoC2E9JRPycS/T1nAyTyt0dLCU3Z41Ik9Awcsmxoq8kuXkDzDyBpyOcMDDRJ4WGQdDPGcKwh1ayrtdbenIcO7EW1klsjljV6QeoCOnMjNGyTROmRG9yblCfzkB+YKFGTwgKBO5abTHeNaVDDoLCgbPBdtUpng9OZwbSCVGxJirMYUdIh\n\nroot:ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC4TjMhQC0wYBDb5dDXnZ9cR4MGVCUQ0qOi3hUuINkxDz+cnz2NX7BFR5RspP8sSWOfy/BEwAiZ4zSG3ch9sz7Oh1RSWNCyGknyyurZGsyAaDb3OTMmETv3KKwH1fk5D2cWrGxNqhNC5BWei1l0srkQEHAJwvv9OlV4MCBZ/RtTTosrxt+K7W/unI+WlZcy4p42wU6MoaPAMHSxJdDeyCdGLCJ1R3c3m88rTstrh5xOCzTYixRhlSvvaILiK9+GnnTgZgavCtnEU1a0ZylGuFl3B196kQLhtQGtFptyTO0SJpRhWcRHYuW1aiywH/v8um1l4EfXsi1DyqFowzCfak6V\n\n"
			}
			]
		},
		"selfLink": "https://www.googleapis.com/compute/v1/projects/ivory-haven-742/zones/asia-east1-c/instances/supersonic-harrenhal-host-55",
		"scheduling": {
			"onHostMaintenance": "MIGRATE",
			"automaticRestart": true
		}
	}
	`

	getOperationResponse = `
	{
		"kind": "compute#operation",
		"id": "8565776068347659798",
		"name": "operation-1415261252844-5072c303d79e1-1d2317ca-32d90950",
		"zone": "https://clients6.google.com/compute/v1/projects/ivory-haven-742/zones/asia-east1-c",
		"operationType": "insert",
		"targetLink": "https://clients6.google.com/compute/v1/projects/ivory-haven-742/zones/asia-east1-c/instances/robust-rottweiler-host-50",
		"targetId": "3414824902925138885",
		"status": "DONE",
		"user": "808587748637-3l83fpdqc40umsggaicmv2da62ikdf0d@developer.gserviceaccount.com",
		"progress": 100,
		"insertTime": "2014-11-06T00:07:33.472-08:00",
		"startTime": "2014-11-06T00:07:33.958-08:00",
		"endTime": "2014-11-06T00:08:11.240-08:00",
		"selfLink": "https://www.googleapis.com/compute/v1/projects/ivory-haven-742/zones/asia-east1-c/operations/operation-1415261252844-5072c303d79e1-1d2317ca-32d90950"
	}
	`
)
