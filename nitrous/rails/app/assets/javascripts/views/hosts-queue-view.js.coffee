App.HostsQueueView = Em.View.extend
  elementId: 'hosts-queue-view'
  classNames: ['ui', 'card']

  wufooURL: (->
    # NOTE: this is hardcoded and will break if the wufoo form changes
    # Reference: http://help.wufoo.com/articles/en_US/SurveyMonkeyArticleType/URL-Modifications
    "https://actionio.wufoo.com/forms/m12kgvzr15pr6pm/def/field227=#{App.user.get('username')}"
  ).property 'App.user.username'
