#= require ./socket/socket

App.EventsSocket = Lib.Socket.extend
  init: ->
    @_super.apply this, arguments
    @_addEventHandlers()

  connect: ->
    origin = location.origin.replace('http', 'ws')
    token = encodeURIComponent(App.get('token'))
    @set 'url', "#{origin}/api/v0/events?token=#{token}"
    @_super.apply this, arguments

  didOpen: (ev) ->
    @_super.apply this, arguments

  didReceiveMessage: (ev) ->
    console.log 'message', ev
    msg = JSON.parse(ev.data)
    @trigger msg.name, msg.data
    @_super.apply this, arguments

  _addEventHandlers: ->
    @on 'image.pulling', (data) ->
      return unless image = App.Image._findInCache(data.id)
      if progress = data.progress
        progress = JSON.parse(progress)
        if progress.status
          progress.status += "..."
        if bar = progress.progress
          if eta = bar.match(/^\[.*\]\s+(.*)$/)?[1]
            progress.status = "Downloading... #{eta}"
        image.set 'progress', progress

    @on 'image.pull_failed', (data) ->
      return unless image = App.Image._findInCache(data.id)
      image.set 'progress', null

    @on 'image.pulled', (data) ->
      return unless image = App.Image._findInCache(data.id)
      image.load(data)
      image.set 'progress', null
