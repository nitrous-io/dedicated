#= require ./model
attr = SM.attr

App.FreeQueue = klass = App.Model.extend
  id: attr('number', serialize: false)
  containerTemplateSlug: attr('string', as: 'container_template_slug', serialize: false)
  containerName: attr('string', as: 'container_name', serialize: false)
  hostName: attr('string', as: 'host_name', serialize: false)
  regionSlug: attr('string', as: 'region_slug', serialize: false)
  state: attr('string', as: 'state', serialize: false)

  template: attr(serialize: false)

klass.reopenClass
  url: '/free_queue'
  primaryKey: 'id'
  singular: true
  rootKey: 'free_queue'

  load: ->
    Em.RSVP.Promise.cast(Em.$.ajax(
      type: 'GET'
      url: "/api/v0/free_queue"
      headers: { Authorization: "Bearer #{App.get 'token'}" }
      cache: false
      contentType: false
    )).then (json) =>
      fq = App.FreeQueue.create()
      fq.load(json.free_queue)
      tmpl = App.Template.create()
      tmpl.load(json.template)
      fq.set "template", tmpl
      App.set "freeQueue", fq
      fq
