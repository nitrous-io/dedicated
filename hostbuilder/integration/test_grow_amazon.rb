#!/usr/bin/env ruby
require "rubygems"
require "bundler/setup"
require "httpclient"
require "json"

client = HTTPClient.new
client.receive_timeout = 10000

id = ARGV[0]

params = {
  name: 'testgrow',
  region_id: 'ap-southeast-1',
  source_ami: 'ami-904e89f3',
  subnet_id: 'subnet-1d77896a',
  security_group_id: 'sg-b9cd64dc',
  instance_type: 't2.micro',
}

resp = client.post "http://localhost:6666/pools/grow", {
  provider: "amazon",
  config: JSON.generate(params)
}
p resp
