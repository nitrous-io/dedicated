Rails.application.routes.draw do
  get '/app' => 'web_app#index'
  get '/ide' => 'web_app#ide'
  get '/token_login' => 'web_app#token_login'

  get '/features' => 'pages#features'
  get '/pricing' => 'pages#pricing'
  get '/company' => 'pages#company'
  get '/press' => 'pages#press'

  get '/flatiron' => 'pages#flatiron'
  get '/education' => 'pages#education'
  get '/agencies' => 'pages#agencies'
  get '/enterprises' => 'pages#enterprises'
  get '/orgs' => 'pages#orgs'
  get '/vpc' => 'pages#vpc'

	get '/awsactivate' => 'pages#awsactivate'
	# hotfixing old link
	get '/partners/:partner' => 'pages#awsactivate', as: :partner_shortcut

  get '/jobs' => 'pages#jobs'
  get '/jobs/:job' => 'pages#job', as: 'job'

  get '/teams-pricing' => 'pages#teams_pricing'
  get '/education-pricing' => 'pages#education_pricing'

  get '/stacks' => 'pages#stacks'
  get '/stacks/:stack' => 'pages#stack'

  get '/oauth_authorize' => 'templates#oauth_authorize'

  root 'pages#home'
end
