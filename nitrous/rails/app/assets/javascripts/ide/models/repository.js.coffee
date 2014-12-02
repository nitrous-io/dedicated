#= require ./base-model

App.Repository = klass = App.BaseModel.extend
  box_id: Em.attr()
  name: Em.attr()
  full_name: Em.attr()
  clone_url: Em.attr()
  state: Em.attr()
  private: Em.attr()
  cloned_at: Em.attr()
  created_at: Em.attr()
  scm: Em.attr()
  failure_reason: Em.attr()
  file_to_open: Em.attr()
  download_branch: Em.attr()

  language: null

  box: (->
    App.Box.findById @get('box_id')
  ).property 'box_id'

  displayName: (->
    if @get('author') && @get('shortName')
      "#{@get 'author'}/#{@get 'shortName'}"
  ).property 'author', 'shortName'

  shortName: (->
    tokens = @get 'tokenizedCloneURL'
    tokens?.name
  ).property 'clone_url'

  author: (->
    tokens = @get 'tokenizedCloneURL'
    tokens?.author
  ).property 'clone_url'

  failed: (->
    @get('state') == 'failed'
  ).property 'state'

  supported: (->
    _.include(['git', 'mercurial'], @get('scm'))
  ).property 'scm'

  isGithub: (->
    @get('tokenizedCloneURL.author')? && @get('clone_url').match(/github\.com/)?
  ).property 'clone_url'

  tokenizedCloneURL: (->
    url = @get('clone_url')
    regex = ///
      ([^/]+)   # Last block before a "/"
      /
      ([^/]+?)  # Last block after a "/"
      (.git)?$  # Optionally ending with ".git"
    ///
    matches = url.match(regex)
    if matches
      {
        author: matches[1]
        name: matches[2]
      }
  ).property 'clone_url'

  identify: (callback) ->
    if @get('isGithub') == true
      author = @get('author')
      name   = @get('shortName')

      githubUser = new Gh3.User(author)
      githubRepo = new Gh3.Repository(name, githubUser)
      githubRepo.fetch (e) =>
        if e?
          @set 'state', 'failed'
        else
          @set 'scm', 'git'
          @set 'state', 'identified'
          @set 'language', githubRepo.language
          @set 'download_branch', githubRepo.default_branch
        callback(e)
    else
      @set 'state', 'unsupported'
      callback null

klass.reopenClass
  rootKey: 'repository'
  collectionKey: 'repositories'
  url: '/repositories'
