App.ContainersNewRoute = App.AuthedRoute.extend
  activate: ->
    App.set 'title', 'New Container'
    App.set 'baseImages', SM.RecordArray.create(model: App.BaseImage).load([
      { id: 1, name: "Go",                 repo: "nitrousio/golang:latest", description: "" }
      { id: 2, name: "Node.js",            repo: "nitrousio/nodejs:latest", description: "" }
      { id: 3, name: "PHP",                repo: "nitrousio/php:latest",    description: "" }
      { id: 4, name: "Python",             repo: "nitrousio/python:latest", description: "" }
      { id: 5, name: "Python with Django", repo: "nitrousio/django:latest", description: "" }
      { id: 6, name: "Ruby",               repo: "nitrousio/ruby:latest",   description: "" }
      { id: 7, name: "Ruby on Rails",      repo: "nitrousio/rails:latest",  description: "" }
      { id: 8, name: "Ubuntu",             repo: "nitrousio/ubuntu:trusty", description: "" }
    ])

  renderTemplate: ->
    @render 'containers/new', outlet: 'content'

  setupController: (controller, model) ->
    @_super.apply this, arguments
    unless controller.get 'saving'
      controller.clear()
