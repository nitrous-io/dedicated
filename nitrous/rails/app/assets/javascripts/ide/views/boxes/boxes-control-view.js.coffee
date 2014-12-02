App.BoxesControlView = Em.View.extend
  freeN2OBinding: Em.Binding.oneWay 'App.currentUser.free_n2o'
  maxN2OBinding: Em.Binding.oneWay 'App.currentUser.max_n2o'

  memoryCost: (->
    unitPrice = App.get 'config.n2o128mbMemory'
    @get('memory') / 128 * unitPrice
  ).property 'memory'

  storageCost: (->
    unitPrice = App.get 'config.n2o250mbStorage'
    @get('storage') / 250 * unitPrice
  ).property 'storage'

  totalCost: (->
    @get('memoryCost') + @get('storageCost')
  ).property 'memoryCost', 'storageCost'

  remainingN2O: (->
    @get('freeN2O') - @get('totalCost')
  ).property 'freeN2O', 'totalCost'

  insufficientN2O: (->
    @get('remainingN2O') < 0
  ).property 'remainingN2O'
