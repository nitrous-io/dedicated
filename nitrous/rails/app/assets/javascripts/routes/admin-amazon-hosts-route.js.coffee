#= require ./admin-route
#= require ../mixins/perform-action-mixin

App.AdminAmazonHostsRoute = App.AdminRoute.extend App.PerformActionMixin,
  provider: null
  activate: ->
    App.set 'title', 'Admin - Amazon Hosts'

  renderTemplate: ->
    @render 'admin/amazon-hosts', outlet: 'content'

  setupController: (controller) ->
    @_super.apply this, arguments
    controller.clear()
