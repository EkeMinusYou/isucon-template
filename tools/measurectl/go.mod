module measurectl

go 1.21

require (
	github.com/google/pprof v0.0.0-20240409012703-83162a5b38cd
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/EkeMinusYou/isucon-template/tools/measurement v0.0.0

replace github.com/EkeMinusYou/isucon-template/tools/measurement => ../measurement
