#= require ./authed-route

App.HostsQueueRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', "You're in the Queue"

  renderTemplate: ->
    @render 'hosts/queue', outlet: 'content'
