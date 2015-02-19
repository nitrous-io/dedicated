App.ApplicationView = Ember.View.extend
  elementId: 'application-view'

  willInsertElement: ->
    @_super.apply this, arguments
    $('html').addClass 'web-app'

  willDestroyElement: ->
    @_super.apply this, arguments
    $('html').removeClass 'web-app'
