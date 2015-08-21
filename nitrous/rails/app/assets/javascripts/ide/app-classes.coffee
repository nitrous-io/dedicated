#= require ./backwards-compatibility
#= require_tree ./mixins
#= require_tree ./lib
#= require ../lib/snackbar
#= require_tree ./models
#= require_tree ./view-models
#= require_tree ./controllers
#= require_tree ./views
#= require ../components/ui-connectivity-indicator-component
#= require ../components/ui-toggle-button-component
#= require ../views/snackbar-view
#= require_tree ./helpers
#= require_tree ../helpers
#= require_tree ./templates
#= require ../templates/components/ui-connectivity-indicator
#= require ../templates/components/ui-toggle-button
#= require ../templates/snackbar
#= require ../lib/event-message-handler
#= require ./code-plugins/manager
#= require_tree ./code-plugins/plugins
#= require_tree ../shared
#= require_tree ./routes
#= require_self
#= require ./routes

# all classes except router are loaded
App.trigger 'load'
