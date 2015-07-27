App.FreeTierRoute = Em.Route.extend
  beforeModel: ->
    App.snackbar.post "We've recently removed the waitlist for our free tier. Please try recreating your workspace."
    @transitionTo 'hosts.new'