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
        findHost(data)?.load(data)

    ['image.created', 'image.removed', 'image.tagged'].forEach (evt) =>
      @on evt, (data) ->
        findAllImages(data)?.reload()

    ['volume.created', 'volume.destroyed'].forEach (evt) =>
      @on evt, (data) ->
        findAllVolumes(data)?.reload()

    ['registry.added', 'registry.removed'].forEach (evt) =>
      @on evt, (data) ->
        findAllRegistries(data)?.reload()

    ['image.building', 'image.pulling'].forEach (evt) =>
      @on evt, (data) ->
        return unless image = findImage(data)
        if progress = parseProgress(data)
          image.set 'progress', progress

    ['image.build_failed', 'image.pull_failed'].forEach (evt) =>
      @on evt, (data) ->
        findImage(data)?.set 'progress', null

    ['image.built', 'image.pulled'].forEach (evt) =>
      @on evt, (data) ->
        return unless image = findImage(data)
        image.load(data)
        image.set 'progress', null

    @on 'image.updated', (data) ->
      findImage(data)?.load(data)

    @on 'image.pushed', (data) ->
      return unless data = data.image
      findImage(data)?.load(data)

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
  App.Image._findInCache(data[pk])

findAllVolumes = (data) ->
  findHostByHostId(data)?.get('volumes')

findAllRegistries = (data) ->
  findHostByHostId(data)?.get('registries')

parseProgress = (data) ->
  return unless progress = data.progress
  progress = JSON.parse(progress)
  if progress.status
    progress.status += "..."
  if progress.stream
    progress.status = progress.stream
  if bar = progress.progress
    eta = bar.match(/^\[.*\]\s+(.*)$/)?[1]
    id = progress.id
    if eta && id
      progress.status = "Downloading #{id}... #{eta}"
  progress
