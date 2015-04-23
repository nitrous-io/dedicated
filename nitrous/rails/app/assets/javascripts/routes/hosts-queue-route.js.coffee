#= require ./authed-route

App.HostsQueueRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', "You've been added to the waitlist"

  renderTemplate: ->
    @render 'hosts/queue', outlet: 'content'
