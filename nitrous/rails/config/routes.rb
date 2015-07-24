Rails.application.routes.draw do
  get '/app' => 'web_app#index'
  get '/ide' => 'web_app#ide'
  get '/token_login' => 'web_app#token_login'

  get '/features' => 'pages#features'
  get '/pricing' => 'pages#pricing'
  get '/company' => 'pages#company'
  get '/press' => 'pages#press'
  get '/plan_changes' => 'pages#plan_changes'
  get '/whatsnew' => 'pages#whatsnew'

  get '/education' => 'pages#education'
  get '/agencies' => 'pages#agencies'
  get '/enterprises' => 'pages#enterprises'
  get '/teams' => 'pages#teams'
  get '/vpc' => 'pages#vpc'
  
  get '/jobs' => 'pages#jobs'
  get '/jobs/:job' => 'pages#job', as: 'job'

  get '/teams-pricing' => 'pages#teams_pricing'
  get '/education-pricing' => 'pages#education_pricing'

  get '/stacks' => 'pages#stacks'
  get '/stacks/:stack' => 'pages#stack'

  get '/oauth_authorize' => 'templates#oauth_authorize'

  root 'pages#home'
end
