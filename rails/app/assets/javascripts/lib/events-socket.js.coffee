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
    msg = JSON.parse(ev.data)
    @trigger msg.name, msg.data
    @_super.apply this, arguments

  _addEventHandlers: ->
    #"image.updated"
    #"image.building"
    #"image.build_failed"
    #"image.tagged"
    #"image.pushed"

    ['container.created', 'container.destroyed'].forEach (evt) =>
      @on evt, (data) ->
        findAllContainers()?.reload()

    ['container.started', 'container.stopped', 'container.restarted', 'container.updated'].forEach (evt) =>
      @on evt, (data) ->
        findContainer(data)?.load(data)

    ['host.created', 'host.destroyed'].forEach (evt) =>
      @on evt, (data) ->
        findAllHosts()?.reload()

    ['host.started', 'host.stopped', 'host.restarted', 'host.updated'].forEach (evt) =>
      @on evt, (data) ->
        if host = findHost(data)
          host.load(data)

    ['image.created', 'image.removed', 'image.built'].forEach (evt) =>
      @on evt, (data) ->
        findAllImages(data)?.reload()

    @on 'image.updated', (data) ->
      @on evt, (data) ->
        findImage(data)?.load(data)

    ['volume.created', 'volume.destroyed'].forEach (evt) =>
      @on evt, (data) ->
        findAllVolumes(data)?.reload()

    ['registry.authenticated', 'registry.removed'].forEach (evt) =>
      @on evt, (data) ->
        findAllRegistries(data)?.reload()

    @on 'image.pulling', (data) ->
      return unless image = findImage(data)
      if progress = data.progress
        progress = JSON.parse(progress)
        if progress.status
          progress.status += "..."
        if bar = progress.progress
          if eta = bar.match(/^\[.*\]\s+(.*)$/)?[1]
            progress.status = "Downloading... #{eta}"
        image.set 'progress', progress

    @on 'image.pull_failed', (data) ->
      findImage(data)?.set 'progress', null

    @on 'image.pulled', (data) ->
      return unless image = findImage(data)
      image.load(data)
      image.set 'progress', null

findAllContainers = ->
  App.get('containers')

findContainer = (data) ->
  pk = App.Container.primaryKey
  findAllContainers()?.findBy(pk, data[pk])

findAllHosts = ->
  App.get('hosts')

findHost = (data) ->
  pk = App.Host.primaryKey
  findAllHosts()?.findBy(pk, data[pk])

findHostByHostId = (data) ->
  findAllHosts()?.findBy('id', data.host_id)

findAllImages = (data) ->
  findHostByHostId(data)?.get('images')

findImage = (data) ->
  pk = App.Image.primaryKey
  findAllImages(data)?.findBy(pk, data[pk])

findAllVolumes = (data) ->
  findHostByHostId(data)?.get('volumes')

findAllRegistries = (data) ->
  findHostByHostId(data)?.get('registries')
