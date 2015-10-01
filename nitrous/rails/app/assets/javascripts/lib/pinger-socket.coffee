#= require ./socket/socket

App.PingerSocket = Lib.Socket.extend
  hostId: null
  interval: 30000 # milliseconds
  timer: null
  firstTime: true

  init: ->
    @_super.apply this, arguments

  connect: ->
    origin = location.protocol.replace('http', 'ws') + '//' + location.host
    token = encodeURIComponent(App.get('token'))
    @set 'url', "#{origin}/api/v0/hosts/#{@get('hostId')}/activity?token=#{token}"
    @_super.apply this, arguments

  didOpen: (ev) ->
    @_super.apply this, arguments

    if @firstTime
      @ping()
      @firstTime = false

    # Clean up any existing timer, so that we only have at most 1 timer.
    if @timer
      Visibility.stop(@timer)

    @timer = Visibility.every(@interval, =>
      @ping()
    )

  ping: ->
    @send('active')
