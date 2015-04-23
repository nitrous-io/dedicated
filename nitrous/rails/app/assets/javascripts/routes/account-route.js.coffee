#= require ./authed-route

App.AccountRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'Account Settings'

  beforeModel: ->
    # FIXME nope
    @transitionTo 'hosts.queue'

  renderTemplate: ->
    @render 'account', outlet: 'content'

  setupController: (controller) ->
    @_super.apply this, arguments
    unless controller.get 'saving'
      controller.clear()
    Mp.track Mp.EventDashboardViewedAccount
