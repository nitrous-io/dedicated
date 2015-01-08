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
