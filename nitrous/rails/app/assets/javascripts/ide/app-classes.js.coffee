#= require ./backwards-compatibility
#= require_tree ./mixins
#= require_tree ./lib
#= require ../lib/snackbar
#= require_tree ./models
#= require_tree ./view-models
#= require_tree ./controllers
#= require_tree ./views
#= require ../views/snackbar-view
#= require ../views/offline-indicator-view
#= require_tree ./helpers
#= require_tree ./templates
#= require ../templates/snackbar
#= require ../templates/offline-indicator
#= require ../lib/event-message-handler
#= require_tree ./routes
#= require_self
#= require ./routes

# all classes except router are loaded
App.trigger 'load'
