App.BaseModel = klass = Em.Model.extend App.Validatable,
  id: Em.attr()
  saving: Em.computed.alias 'isSaving'
  isDeleting: false
  deleting: Em.computed.alias 'isDeleting'

  makeRequest: (action, method='GET', params) ->
    promise = @constructor.adapter.makeRequest(this, action, method, params)
    promise.then undefined, (e) =>
      if e.status == 422 && e.responseText
        @set 'errors', JSON.parse(e.responseText).errors
    promise

  loadHash: (hash) ->
    @load hash[Em.get @constructor, 'primaryKey'], hash

  save: ->
    self = this
    @validate()
    if !Em.empty(@get('errors'))
      return new Em.RSVP.Promise((resolve, reject) ->
        reject(self)
      )
    promise = @_super()
    promise.then undefined, (e) =>
      if e.status == 422 && e.responseText
        @set 'errors', JSON.parse(e.responseText).errors
      @set 'isSaving', false
    promise

  deleteRecord: ->
    @set 'isDeleting', true
    promise = @_super()
    promise.then =>
      @set 'isDeleting', false
    , (e) =>
      @set 'isDeleting', false
    promise

klass.reopenClass
  adapter: App.RESTAdapter.create()
  camelizeKeys: false # too much work

  all: ->
    @_findAllRecordArray ?= Em.RecordArray.create(modelClass: this, content: Em.A())

  preload: (hashes) ->
    recordArray = @all()
    recordArray.load this, hashes
    recordArray

  _findFetchById: (id) ->
    @_super.apply(this, arguments) if id?
