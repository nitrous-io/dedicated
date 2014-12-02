App.HackView = Em.View.extend
  templateName: 'templates/hack'
  classNames: ['app_page']
  elementId: 'hack-view'

  didInsertElement: ->
    @_super()
    App.util.mixpanel 'Viewed Box Picker for Cloning Repo'
