#= require_tree ./mixins
#= require_tree ./lib
#= require_tree ./models
#= require_tree ./view-models
#= require_tree ./controllers
#= require_tree ./components
#= require_tree ./views
#= require_tree ./helpers
#= require_tree ./templates
#= require_tree ./routes
#= require_tree ./shared
#= require_self
#= require routes

# all classes except router are loaded
App.trigger 'load'
