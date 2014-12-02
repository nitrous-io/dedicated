App.BoxDomainsView = Em.View.extend
  templateName: 'templates/box/domains'
  elementId: 'box-domains-view'
  classNames: ['app_page', 'edit', 'settings']

  didInsertElement: ->
    @_super()
    App.util.mixpanel 'Viewed Custom Domains', box_id: @get('controller.id')

  didClickSave: (event) ->
    @get('controller').save()

  didClickCancel: (event) ->
    @get('controller').cancel()
