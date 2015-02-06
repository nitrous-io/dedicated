#= require ./model
attr = SM.attr

App.LiveHost = App.Model.extend
  id: attr('string')
  provider: attr()
  ipAddress: attr(as: 'ip_address')
  freighterdPort: attr(as: 'freighterd_port')
  state: attr()

  isRunning: (->
    @get('state') == 'Running'
  ).property 'state'

  providerUrl: (->
    if @get('provider') == 'digitalocean'
      "https://cloud.digitalocean.com/droplets/#{@get('id')}"
  ).property 'id'
