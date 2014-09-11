#= require ./socket/socket

App.EventSocket = Lib.Socket.extend
  connect: ->
    origin = location.origin.replace('http', 'ws')
    token = encodeURIComponent(App.get('token'))
    @set 'url', "#{origin}/api/v0/events?token=#{token}"
    @_super.apply this, arguments

  didOpen: (ev) ->
    @_super.apply this, arguments
