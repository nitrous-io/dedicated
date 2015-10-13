#= require ./model
attr = SM.attr

App.Repository = klass = App.Model.extend
  name: attr()
  gitUrl: attr(as: 'git_url')
  private: attr()

klass.reopenClass
  url: '/github/repos'
  collectionKey: 'repos'
