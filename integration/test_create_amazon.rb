#!/usr/bin/env ruby
require "rubygems"
require "bundler/setup"
require "httpclient"
require "json"

client = HTTPClient.new

params = {
  source_ami: "ami-167b1426",
  region: "us-west-2",
  security_group: {
    id: "sg-9b4c04ab",
    name: "box_host"
  },
  availability_zone: "us-west-2a",
  instance_type: "t1.micro",
  public_keys: [
    File.read("#{ENV['HOME']}/.ssh/id_rsa.pub")
  ],
}

resp = client.post "http://localhost:6666/host", {
  provider: "amazon",
  config: JSON.generate(params)
}
p resp
