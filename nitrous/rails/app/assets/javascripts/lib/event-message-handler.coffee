#= require ./events-socket

App.EventMessageHandler = App.EventsSocket.extend
  init: ->
    @_super.apply this, arguments
    @_addEventHandlers()

  _addEventHandlers: ->
    ['container.created', 'container.destroyed', 'host.restarted'].forEach (evt) =>
      @on evt, (data) ->
        findAllContainers()?.reload()

    ['container.starting', 'container.started','container.stopping', 'container.stopped', 'container.restarting', 'container.restarted',
     'container.updated', 'container.reconfiguring', 'container.reconfigured', 'container.reconfigure_failed'].forEach (evt) =>
      @on evt, (data) ->
        containerData = data.container
        if container = findContainer(containerData)
          container.eventOccurred(evt)
          container.load(containerData)
          container.loadConfig()

    ['hosts.refreshed', 'host.created', 'host.create_failed', 'host.destroyed'].forEach (evt) =>
      @on evt, (data) ->
        findAllHosts()?.reload()

    @on 'host.creating', (data) =>
      return unless host = findHostBySlug(data)
      if progress = parseProgress(data)
        host.set 'progress', progress

    ['host.create_failed', 'host.created'].forEach (evt) =>
      @on evt, (data) =>
        hostData = data.host
        return unless host = findHostBySlug(hostData)
        host.load(hostData)
        host.set 'progress', null

    ['host.restarted', 'host.restarting', 'host.updated',
      'host.stopping', 'host.stopped', 'host.starting', 'host.started',
      'host.software_updating', 'host.software_update_failed',
      'host.respeccing', 'host.respec_failed'].forEach (evt) =>
      @on evt, (data) ->
        hostData = data.host
        return unless host = findHost(hostData)
        host.load(hostData)

    ['host.software_updated', 'host.respecced'].forEach (evt) =>
      @on evt, (data) ->
        hostData = data.host
        return unless host = findHost(hostData)
        host.load(hostData)
        host.reload()
        host.fetchStats()

    ['volume.created', 'volume.destroyed'].forEach (evt) =>
      @on evt, (data) ->
        volumeData = data.volume
        findAllVolumes(volumeData)?.reload()

    ['registry.added', 'registry.removed'].forEach (evt) =>
      @on evt, (data) ->
        registryData = data.registry
        findAllRegistries(registryData)?.reload()

    ['image.created', 'image.removed', 'image.tagged'].forEach (evt) =>
      @on evt, (data) ->
        imageData = data.image
        findAllImages(imageData)?.reload()

    ['image.building', 'image.pulling'].forEach (evt) =>
      @on evt, (data) ->
        return unless image = findImage(data)
        if progress = parseProgress(data)
          image.set 'progress', progress

    ['collaborator.added', 'collaborator.removed'].forEach (evt) =>
      @on evt, (data) ->
        containerData = data.container
        if container = findContainer(containerData)
          container.get("collaborators").reload()
          return if container.get("userId") == App.get('user.id')

        if App.get('sharedContainers')
          App.get('sharedContainers').reload()
        else
          App.set('sharedContainers', App.SharedContainer.fetch().value)

        if evt == "collaborator.added" && data.container.user_id != App.get('user.id')
            App.snackbar.post("#{data.initiated_by.username} has added to you to #{data.container.slug}")

    @on 'image.build_failed', (data) ->
      findImage(data)?.set 'progress', null

    @on 'image.built', (data) ->
      imageData = data.image
      return unless image = findImage(imageData)
      image.load(imageData)
      image.set 'progress', null

    ['image.pulled', 'image.pull_failed'].forEach (evt) =>
      @on evt, (data) ->
        imageData = data.image
        return unless image = findImage(imageData)
        image.load(imageData)
        image.set 'updating', false
        image.set 'progress', null

    @on 'image.updated', (data) ->
      imageData = data.image
      findImage(imageData)?.load(imageData)

    @on 'image.pushed', (data) ->
      imageData = data.image
      return unless imageData && (image = findImage(imageData))
      image.load(imageData)
      image.set 'pushing', false

    @on 'invoice.updated', (data) ->
      App.refreshBillingProfile()
      if (data.invoice && inv = App.get('invoices').findBy('id', data.invoice.id))
        inv.load(data.invoice)
        inv.set('awaitingUpdate', false)
      else
        App.refreshInvoices()

    @on 'subscription.updated', (data) ->
      if (data.subscription && sub = App.get('subscriptions').findBy('id', data.subscription.id))
        sub.load(data.subscription)
      else
        App.refreshSubscriptions()

    @on 'subscription.deleted', (data) ->
      App.refreshSubscriptions()

findAllContainers = ->
  App.get('containers')

findAllSharedContainers = ->
  App.get('sharedContainers')

findContainer = (data) ->
  pk = App.Container.primaryKey
  c = findAllContainers()?.findBy(pk, data[pk])
  if c
    return c
  return findAllSharedContainers()?.findBy(pk, data[pk])

findAllHosts = ->
  App.get('hosts')

findHost = (data) ->
  pk = App.Host.primaryKey
  findAllHosts()?.findBy(pk, data[pk])

findHostBySlug = (data) ->
  App.Host._findInCache(data.slug)

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
  App.trigger("progressReceived", progress)
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
