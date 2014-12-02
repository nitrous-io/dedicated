App.N2oChangePlanRoute = Em.Route.extend
  beforeModel: ->
    if App.get('currentUser.subscription_pending')
      @transitionTo 'n2o.index'
      Em.run.scheduleOnce 'afterRender', this, ->
        App.set 'flashMessage', type: 'alert-success', text: 'We are still processing your new subscription, you should be receiving an email shortly.'
    else if App.get('currentUser.isDelinquent')
      @transitionTo 'account'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    controller.initialize()

  deactivate: ->
    @controller.reset()
