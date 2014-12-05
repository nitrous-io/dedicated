#= require ./authed-route

App.AppSnapshotsRoute = App.AuthedRoute.extend
  renderTemplate: ->
    @render 'app/snapshots', into: 'app'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    App.set 'title', model.get('displayName') + ' › Snapshots'
