require 'sequel'

NITROUS_DB = Sequel.connect(ENV['NITROUS_PG_URL'])
PUBSTORM_DB = Sequel.connect(ENV['PUBSTORM_PG_URL'])
