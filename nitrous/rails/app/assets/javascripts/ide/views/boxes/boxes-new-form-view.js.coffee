#= require ./boxes-control-view

App.BoxesNewFormView = App.BoxesControlView.extend
  templateName: 'templates/boxes/new-form'
  elementId: 'boxes_form_view'

  snapshotsBinding: 'App.currentUser.completedSnapshots'

  memoryBinding: 'controller.content.memory'
  storageBinding: 'controller.content.storage'

  didInsertElement: ->
    @_super()
    @preselectRegion()
    @_setRandomName()

  createBoxButtonDisabled: (->
    @get('insufficientN2O') || @get('controller.content.saving')
  ).property 'controller.content.saving', 'insufficientN2O'

  didSelectBoxTemplate: (->
    if box = @get 'controller.content'
      if box.get 'box_template_id'
        box.set 'source_snapshot_id', null
  ).observes 'controller.content.box_template_id'

  didSelectSourceSnapshot: (->
    if box = @get 'controller.content'
      if box.get 'source_snapshot_id'
        box.set 'box_template_id', null
      else unless box.get 'box_template_id'
        box.set 'box_template_id', App.get('boxTemplates.firstObject.id')
  ).observes 'controller.content.source_snapshot_id'

  preselectRegion: (->
    if box = @get 'controller.content'
      return if box.get 'region_id' # Don't interfere with user's selection.
      if regionId = App.get 'closestRegionId'
        box.set 'region_id', regionId
        return
      App.Region.fetchClosestRegion()
  ).observes 'App.closestRegionId'

  didClickCreateBox: ->
    @get('controller').save()

  didClickRandom: ->
    @_setRandomName()

  didClickCancel: ->
    @get('controller').cancel()

  _setRandomName: ->
    name = App.RandomBoxNameGenerator.generate()
    @set('controller.content.name', name)

App.BoxNameTextField = Em.TextField.extend
  click: ->
    unless @get 'controller.boxNameIsDirty'
      @$().select()

  keyPress: ->
    @set 'controller.boxNameIsDirty', true
