#= require ./model
attr = SM.attr

App.ContainerKey = klass = App.Model.extend
  fingerprint: attr(serialize: false)
  key: attr()

  # FIXME: partly duplicates name property in models/user-key
  name: (->
    fingerprint = @get 'fingerprint'
    name = @get('key')?.match(/^\s*ssh-(rsa|dss)\s+\S+\s+(\S+)/)?[2] || "Container Key"
    "#{name} (#{fingerprint})"
  ).property 'fingerprint', 'key'
