#= require ./socket/socket

App.PingerSocket = Lib.Socket.extend
  hostId: null
  interval: 30000 # milliseconds
  timer: null

  init: ->
    @_super.apply this, arguments

  connect: ->
    origin = location.protocol.replace('http', 'ws') + '//' + location.host
    token = encodeURIComponent(App.get('token'))
    @set 'url', "#{origin}/api/v0/hosts/#{@get('hostId')}/activity?token=#{token}"
    @_super.apply this, arguments

  didOpen: (ev) ->
    @_super.apply this, arguments

    # Clean up any existing timer, so that we only have at most 1 timer.
    if @timer
      window.clearInterval(@timer)

    @ping()
    @timer = window.setInterval(=>
      @ping()
    , @interval)

  ping: ->
    @send('active')
