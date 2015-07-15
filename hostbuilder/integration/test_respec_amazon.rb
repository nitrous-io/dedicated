#!/usr/bin/env ruby
require "rubygems"
require "bundler/setup"
require "httpclient"
require "json"

client = HTTPClient.new
client.receive_timeout = 10000

if ARGV.length < 2
  puts "Usage: test_respec_amazon.rb <instance ID> <storage size> <instance type>"
  exit 1
end

id = ARGV[0]
storage_size = ARGV[1].to_i
instance_type = ARGV[2]

puts "Respeccing instance #{id}: storage size=#{storage_size}, instance type=#{instance_type}"

params = {
  id: id,
  region_id: "us-west-2",
  storage_size: storage_size,
  disable_eip: false
}
if instance_type != ""
  params[:instance_type] = instance_type
end

resp = client.post "http://localhost:6666/hosts/respec", {
  provider: "amazon",
  config: JSON.generate(params)
}
p resp
