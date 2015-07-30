require 'bundler/setup'
require 'dotenv'
require 'sequel'
require 'ethon'
require 'json'

Dotenv.load
$db = Sequel.connect(ENV['PG_URL'])

sql = <<-SQL
SELECT containers.slug AS container_slug, containers.cid, hosts.slug AS host_slug, hosts.ip_address, hosts.freighterd_port, hosts.secret
    FROM containers INNER JOIN hosts ON containers.host_id = hosts.id
    WHERE containers.terminated_at IS NULL AND hosts.state = 'running'
    LIMIT 100
SQL

ca_file = ENV['CA_PATH']
domain = ENV['NITROUS_DOMAIN']

$db.fetch(sql) do |row|
  # Use libcurl for Server Name Indicator feature with cert file
  host_with_port = "#{row[:container_slug]}.#{domain}:#{row[:freighterd_port]}"
  path = "/containers/#{row[:cid]}"
  resolve = Ethon::Curl.slist_append(resolve, "#{host_with_port}:#{row[:ip_address]}")
  client = Ethon::Easy.new(url: "https://#{host_with_port}#{path}", resolve: resolve, cainfo: ca_file, headers: { "X-Freighter-Secret" =>  row[:secret]})
  client.perform

  if client.response_code == 200
    container = JSON.parse(client.response_body)
    # Check if there is NITROUS_PREVIEW_PORT variable
    preview_port_env = container['container']['env'].detect { |env| env.start_with?('NITROUS_PREVIEW_PORT') }
    preview_port = preview_port_env.split('=').last if preview_port_env

    port_mappings = {}
    container['container']['ports'].each_pair do |exposed_port, mapping_ports|
      if !preview_port.nil? && preview_port.length > 0 && preview_port == exposed_port
        port_mappings['preview'] = mapping_ports.first if mapping_ports.length > 0
      else
        port_mappings[exposed_port] = mapping_ports.first if mapping_ports.length > 0
      end
    end
    puts "UPDATE containers SET port_mappings = '#{JSON.dump(port_mappings)}' WHERE slug = '#{row[:container_slug]}';" unless port_mappings.empty?
  end
end

