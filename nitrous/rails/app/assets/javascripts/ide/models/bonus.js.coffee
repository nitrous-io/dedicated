#= require ./base-model

App.Bonus = klass = App.BaseModel.extend
  invite_friends: Em.attr()
  github: Em.attr()
  twitter: Em.attr()
  facebook: Em.attr()
  linkedin: Em.attr()
  google: Em.attr()
  accepted_invite: Em.attr()
  chrome_app: Em.attr()
  twitter_follow: Em.attr()
  tweet_sent: Em.attr()
  upgrade_account: Em.attr()
  business_survey: Em.attr()
  user_survey_1: Em.attr()

klass.reopenClass
  adapter: null
  rootKey: 'bonus'
