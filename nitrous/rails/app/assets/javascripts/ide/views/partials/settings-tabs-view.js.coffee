App.SettingsTabsView = Em.View.extend
  templateName: 'templates/partials/settings'
  elementId: 'settings_tabs_view'

  activeTab: null
  box: null

  resourcesTabActive: (->
    @get('activeTab') == 'resources'
  ).property 'activeTab'

  domainTabActive: (->
    @get('activeTab') == 'domain'
  ).property 'activeTab'

  snapshotsTabActive: (->
    @get('activeTab') == 'snapshots'
  ).property 'activeTab'

  didClickResources: (event) ->
    unless @get('resourcesTabActive')
      App.get('router').transitionTo 'box.resources', @get('box')

  didClickDomain: (event) ->
    unless @get('domainTabActive')
      App.get('router').transitionTo 'box.domains', @get('box')

  didClickSnapshots: (event) ->
    unless @get('snapshotsTabActive')
      App.get('router').transitionTo 'box.snapshots.index', @get('box')

  didClickCancel: (event) ->
    App.get('router').transitionTo 'boxes.index'
