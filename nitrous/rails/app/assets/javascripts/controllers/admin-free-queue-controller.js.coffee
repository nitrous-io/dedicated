App.AdminFreeQueueController = Em.Controller.extend
  fetchingQueue: false
  queue: null

  clear: ->
    @setProperties
      queue: Em.A()
      fetchingQueue: false
    @refreshQueue()

  refreshQueue: ->
    @set('fetchingQueue', true)

    Em.$.ajax(
      type: 'GET'
      url: '/api/v0/admin/free_queue/pending'
      headers: { Authorization: "Bearer #{App.get 'token'}" }
    )
    .then((data) =>
      queue = data.queue.map (entry)->
        image = App.AdminFreeQueue.newRecord({id: entry.id})
        image.load(entry)
        image
      @set('queue', queue)
      @set('fetchingQueue', false)
    , =>
      @set('fetchingQueue', false)
    )

  actions:
    refresh: ->
      @refreshQueue()

    approve: (record)->
      record.approve().then =>
        @refreshQueue()
      , (err)=>
        App.postError(err, "Failed to approve #{record.get('id')}")
        @refreshQueue()

    reject: (record)->
      record.reject().then =>
        @refreshQueue()
      , (err)=>
        App.postError(err, "Failed to reject #{record.get('id')}")
        @refreshQueue()