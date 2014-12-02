App.Config = Em.Object.extend
  environment: null
  shoverServerURI: null
  firebaseHost: null
  n2o128mbMemory: null
  n2o250mbStorage: null
  stripePublishableKey: null
  paymentsEnabled: null
  maxMemoryMB: null
  maxStorageMB: null
  minMemoryMB: null
  minStorageMB: null

  maxMemoryGB: (->
    maxMemoryMB = @get 'maxMemoryMB'
    if _.isNumber(maxMemoryMB)
      maxMemoryMB / 1024
  ).property 'maxMemoryMB'

  maxStorageGB: (->
    maxStorageMB = @get 'maxStorageMB'
    if _.isNumber(maxStorageMB)
      maxStorageMB / 1000
  ).property 'maxStorageMB'
