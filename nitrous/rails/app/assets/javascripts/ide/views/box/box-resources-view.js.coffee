#= require ../boxes/boxes-control-view

App.BoxResourcesView = App.BoxesControlView.extend
  templateName: 'templates/box/resources'
  classNames: ['app_page', 'edit', 'settings']
  elementId: 'box-resources-view'

  startingMemoryCost: null
  startingStorageCost: null

  memoryBinding: 'controller.content.memory'
  storageBinding: 'controller.content.storage'
  memoryUsageBinding: 'controller.content.memory_usage'
  storageUsageBinding: 'controller.content.storage_usage'
  nameBinding: 'controller.content.name'

  didInsertElement: ->
    if box = @get 'controller.content'
      App.util.mixpanel 'Viewed Box Edit', box_id: box.get('id')
      box.fetchBoxInfo()
    @set 'startingMemoryCost', @get('memoryCost')
    @set 'startingStorageCost', @get('storageCost')

  memoryCostChange: (->
    0 - (@get('memoryCost') - @get('startingMemoryCost'))
  ).property 'memoryCost', 'startingMemoryCost'

  storageCostChange: (->
    0 - (@get('storageCost') - @get('startingStorageCost'))
  ).property 'storageCost', 'startingStorageCost'

  totalCostChange: (->
    @get('memoryCostChange') + @get('storageCostChange')
  ).property 'memoryCostChange', 'storageCostChange'

  remainingN2O: (->
    @get('freeN2O') + @get('totalCostChange')
  ).property 'freeN2O', 'totalCostChange'

  insufficientStorage: (->
    @get('storage') <= @get('storageUsage')
  ).property 'storage', 'storageUsage'

  waitingStorageUsage: (->
    !@get('storageUsage')?
  ).property 'storageUsage'

  saveChangesButtonDisabled: (->
    @get('insufficientN2O') || @get('controller.content.saving') || @get('waitingStorageUsage') || @get('insufficientStorage')
  ).property 'controller.content.saving', 'insufficientN2O', 'storageUsage'

  memoryUsageDetails: (->
    memoryUsage = @get 'memoryUsage'
    memoryAllocated = @get 'memory'

    if memoryUsage && memoryAllocated
      if memoryUsage == -1
        "N/A (not running)"
      else
        "#{memoryUsage.toFixed(1)}MB of #{memoryAllocated}MB used"
  ).property 'memoryUsage', 'memory'

  storageUsageDetails: (->
    storageUsage = @get 'storageUsage'
    storageAllocated = @get 'storage'

    if storageUsage && storageAllocated
      "#{storageUsage.toFixed(1)}MB of #{storageAllocated}MB used"
  ).property 'storageUsage', 'storage'

  didClickChangeBox: (event) ->
    @get('controller').save()

  didClickCancel: (event) ->
    @get('controller').cancel()
