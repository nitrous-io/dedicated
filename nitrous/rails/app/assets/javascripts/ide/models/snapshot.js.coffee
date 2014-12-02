#= require ./base-model

App.Snapshot = klass = App.BaseModel.extend
  name: Em.attr()
  state: Em.attr()
  user_id: Em.attr()
  box_id: Em.attr()
  public: Em.attr()
  created_at: Em.attr(Date)
  completed_at: Em.attr(Date)

  validations:
    name: (v) -> "can't be blank" unless v && v.length > 0

  box: (->
    App.Box.findById @get('box_id')
  ).property 'box_id'

  description: (->
    if @get 'box'
      "#{@get 'name'} of box #{@get 'box.name'} (created #{@get 'humanizedCreatedAt'})"
    else
      "#{@get 'name'} created #{@get 'humanizedCreatedAt'}"
  ).property 'name', 'humanizedCreatedAt', 'box'

  humanizedCreatedAt: App.util.humanizedTimeProperty('created_at')
  humanizedCompletedAt: App.util.humanizedTimeProperty('completed_at')

  queued: (->
    @get('state') == 'queued'
  ).property 'state'

  initiated: (->
    @get('state') == 'initiated'
  ).property 'state'

  completed: (->
    @get('state') == 'completed'
  ).property 'state'

  failed: (->
    @get('state') == 'failed'
  ).property 'state'

  deleted: (->
    @get('state') == 'deleted'
  ).property 'state'

klass.reopenClass
  rootKey: 'snapshot'
  collectionKey: 'snapshots'
  url: '/snapshots'

klass.Collection = Em.ArrayProxy.extend Em.SortableMixin,
  sortProperties: ['created_at']
  sortAscending: false
