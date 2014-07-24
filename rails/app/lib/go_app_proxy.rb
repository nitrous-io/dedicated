class GoAppProxy < Rack::Proxy
  def rewrite_env(env)
    env['HTTP_HOST'] = 'localhost:3333'
    env
  end
end
