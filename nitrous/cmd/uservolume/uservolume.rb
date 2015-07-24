require 'bundler/setup'
require 'dotenv'
require 'sequel'
require 'aws-sdk'

Dotenv.load
$db = Sequel.connect(ENV['PG_URL'])
credentials = Aws::Credentials.new(ENV['AWS_ACCESS_KEY_ID'], ENV['AWS_SECRET_ACCESS_KEY'])

# The index is from `regions` table
clients = {}
clients[5] = Aws::EC2::Client.new(region: 'us-west-2',      credentials: credentials)
clients[6] = Aws::EC2::Client.new(region: 'us-east-1',      credentials: credentials)
clients[7] = Aws::EC2::Client.new(region: 'eu-west-1',      credentials: credentials)
clients[8] = Aws::EC2::Client.new(region: 'ap-southeast-1', credentials: credentials)

host_ids = Hash.new { |hash, key| hash[key] = {} }
$db.fetch("SELECT id, provider_identifier, region_id FROM hosts WHERE terminated_at IS NULL AND provider_storage_identifier IS NULL") do |row|
  id = row[:id]
  region_id = row[:region_id].to_i
  instance_id = row[:provider_identifier]
  host_ids[region_id][instance_id] = id
end

host_ids.each_pair do |region_id, instances|
  client = clients[region_id]

  (instances.count / 100.0).ceil.times do |i|
    first = i * 100
    last = (i+1) * 100 - 1

    begin
      results = client.describe_instances({ instance_ids: instances.keys[first..last] })
      results.reservations.each do |reservation|
        instance_info = reservation.instances.first
        if instance_info.nil?
          puts "Invalid instance: #{reservation.inspect}"
          next
        end

        instance_id = instance_info.instance_id
        root_volume_id = instance_info.block_device_mappings[0].ebs.volume_id
        host_id = instances[instance_id]

        scope = $db[:hosts].where(id: host_id)
        puts "Update host(#{instance_id})'s volume ID to #{root_volume_id}"
        scope.update(provider_storage_identifier: root_volume_id)
      end
    rescue Aws::EC2::Errors::ServiceError
      puts "Failed to update some hosts"
    end
  end
end
