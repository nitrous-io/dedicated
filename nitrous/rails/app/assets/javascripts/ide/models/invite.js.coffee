#= require ./base-model

App.Invite = klass = App.BaseModel.extend
  email: Em.attr()
  state: Em.attr()
  created_at: Em.attr(Date)
  n2o_earned: Em.attr(Number)

  humanizedState: (->
    switch state = @get 'state'
      when 'activated'
        "sent"
      else
        state
  ).property 'state'

  humanizedN2OEarned: (->
    if n2o_earned = @get 'n2o_earned'
      "+#{n2o_earned}"
    else
      ""
  ).property 'n2o_earned'

  humanizedCreatedAt: App.util.humanizedTimeProperty('created_at')

  validations:
    email: (v) -> "can't be blank" unless v && v.length > 0

klass.reopenClass
  rootKey: 'invite'
  collectionKey: 'invites'
  url: '/invites'
