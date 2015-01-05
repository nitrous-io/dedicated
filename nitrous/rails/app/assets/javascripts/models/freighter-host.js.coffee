#= require ./model
attr = SM.attr

App.FreighterHost = App.Model.extend
  id: attr()
  provider: attr()
  ipAddress: attr(as: 'ip_address')
  freighterdPort: attr(as: 'freighterd_port')
  state: attr()
