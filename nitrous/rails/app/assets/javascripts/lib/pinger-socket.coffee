#= require ./socket/socket

App.PingerSocket = Lib.Socket.extend
  hostId: null
  interval: 30000 # milliseconds
  timer: null
  listener: null
  ackThreshold: 120000 # milliseconds
  firstTime: true
  alert: true

  lastSent: null
  lastAck: null

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

    @startPinging()
    @handleVisibilityChange()

  didReceiveMessage: (ev) ->
    msg = ev.data
    if found = msg.match(/^ok\|(\d+)/)
      @set('lastAck', found[1])
    @_super.apply this, arguments

  didFail: (ev) ->
    console.warn 'Ping WebSocket encountered an error'
    # Artificially simulate a ping so that we will display an error message
    # after ackThreshold has passed.
    @set('lastSent', Date.now())

  handleVisibilityChange: ->
    return if @listener
    @listener = Visibility.change((e, state) =>
      if Visibility.hidden()
        @stopPinging()
      else
        @startPinging()
    )

  startPinging: ->
    # Clean up any existing timer, so that we only have at most 1 timer.
    @stopPinging()
    @timer = Visibility.every(@interval, =>
      @ping()
    )

  stopPinging: ->
    return if !@timer?
    Visibility.stop(@timer)
    @set('lastSent', null)
    @set('lastAck', null)

  ping: ->
    timestamp = Date.now()
    @send("active|#{timestamp}")
    @set('lastSent', timestamp)

  observeAcks: (->
    return if !@get('lastSent')

    lastSent = parseInt(@get('lastSent'), 10)
    if !@get('lastAck')
      # Never acknowledged, compare to current time.
      diff = lastSent - Date.now()
    else
      diff = lastSent - parseInt(@get('lastAck'), 10)

    if diff <= @ackThreshold
      if @snackbarItem
        App.snackbar.fadeOutItem(@snackbarItem)
        @snackbarItem = null
        Mp.track("User activity ping restored")
      return

    # do not alert user unless controlled to
    unless @alert
      return

    console.warn "Ping not acknowledged for #{diff}ms"

    msg = "There are issues detecting activity on your IDE. Your workspace may shut down unexpectedly. Please contact support with a screenshot of your JavaScript console."
    if !@snackbarItem
      @snackbarItem = App.snackbar.post(msg, sticky: true)
      Mp.track("User activity ping failed")
  ).observes 'lastSent', 'lastAck'
