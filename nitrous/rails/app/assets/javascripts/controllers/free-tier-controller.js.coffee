App.FreeTierController = Em.ObjectController.extend
  # skip allows informing this controller
  # that the model has recently been fetched
  queryParams: ['skip']
  skip: null

  run: ->
    if @get('skip')
      @_redirect()
    else
      App.FreeQueue.load().then =>
        @_redirect()

  _redirect: ->
    fq = App.get "freeQueue"
    unless fq
      @transitionToRoute 'home'
      return

    if fq.get("state") != "approved"
      @transitionToRoute 'hosts.queue'
      return

    hst = App.Host.create()
    hst.set 'name', fq.get 'hostName'
    hst.set 'planSlug', 'ebs-free'
    hst.set 'regionSlug', fq.get 'regionSlug'
    App.set 'pendingHost', hst

    App.set 'pendingContainerData', Em.Object.create
      name: fq.get("containerName")
      selectedTemplate: fq.get("template")

    @transitionToRoute 'hosts.new', queryParams: { pendingContainer: true }