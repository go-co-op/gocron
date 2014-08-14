## goCron: A Golang Job Scheduling Package.

goCron is a Golang job scheduling package which lets you run Go functions periodically at pre-determined interval using a simple, human-friendly syntax.

goCron is a Golang implementation of Ruby module [clockwork](<https://github.com/tomykaira/clockwork>) and Python job scheduling package [schedule](<https://github.com/dbader/schedule>), and personally, this package is my first Golang program, just for fun and practice.

See also this two great articles:
* [Rethinking Cron](http://adam.heroku.com/past/2010/4/13/rethinking_cron/)
* [Replace Cron with Clockwork](http://adam.heroku.com/past/2010/6/30/replace_cron_with_clockwork/)

Back to this package, you could just use this simple API as below, to run a cron scheduler.

``` golang
import (
	"fmt"
	"github.com/jasonlvhit/gocron"
)

func task(){
	fmt.Println("I am running...")
}

func main(){
	gocron.Every(10).Seconds().Do(task)
	gocron.Every(1).Monday().At("12:38").Do(task)
	gocron.Start()
}
```
and full test cases and document will be coming soon.

Once again, thanks to the great works of Ruby clockwork and Python schedule package. BSD license is used, see the file License for detail.

Hava fun!
