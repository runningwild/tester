package main

import (
  "fmt"
  gl "github.com/chsc/gogl/gl21"
  "github.com/runningwild/glop/gin"
  "github.com/runningwild/glop/gos"
  "github.com/runningwild/glop/gui"
  "github.com/runningwild/glop/render"
  "github.com/runningwild/glop/sprite"
  "github.com/runningwild/glop/system"
  "code.google.com/p/freetype-go/freetype/truetype"
  "io/ioutil"
  "os"
  "path/filepath"
  "runtime"
  // "runtime/pprof"
  "sort"
  "time"
)

type loadResult struct {
  anim *sprite.Sprite
  err  error
}

var (
  sys        system.System
  datadir    string
  key_map    KeyMap
  action_map KeyMap
  loaded     chan loadResult
  dict       *gui.Dictionary
  error_msg  *gui.TextLine
  commands   map[string]Command
)

type Command struct {
  Cmd  string
  Me   []string
  You  []string
  Sync string
}

func init() {
  runtime.LockOSThread()
  runtime.GOMAXPROCS(2)
  datadir = filepath.Join(os.Args[0], "..", "..")
  var key_binds KeyBinds
  LoadJson(filepath.Join(datadir, "bindings.json"), &key_binds)
  key_map = key_binds.MakeKeyMap()
  key_binds = make(map[string]string)
  LoadJson(filepath.Join(datadir, "actions.json"), &commands)
  for name, cmd := range commands {
    key_binds[name] = cmd.Cmd
  }
  action_map = key_binds.MakeKeyMap()
  loaded = make(chan loadResult)
}

func GetStoreVal(key string) string {
  var store map[string]string
  LoadJson(filepath.Join(datadir, "store"), &store)
  if store == nil {
    store = make(map[string]string)
  }
  val := store[key]
  return val
}

func SetStoreVal(key, val string) {
  var store map[string]string
  path := filepath.Join(datadir, "store")
  LoadJson(path, &store)
  if store == nil {
    store = make(map[string]string)
  }
  store[key] = val
  SaveJson(path, store)
}

type spriteBox struct {
  gui.EmbeddedWidget
  gui.BasicZone
  gui.NonThinker
  gui.NonResponder
  gui.NonFocuser
  gui.Childless
  s       *sprite.Sprite
  r, g, b float64
  top     bool
}

func makeSpriteBox(s *sprite.Sprite) *spriteBox {
  var sb spriteBox
  sb.EmbeddedWidget = &gui.BasicWidget{CoreWidget: &sb}
  sb.Request_dims = gui.Dims{300, 300}
  sb.r, sb.g, sb.b = 0.2, 0.1, 0.4
  return &sb
}
func (sb *spriteBox) String() string {
  return "sprite box"
}
func (sb *spriteBox) Draw(region gui.Region) {
  gl.Disable(gl.TEXTURE_2D)
  gl.Color4d(sb.r, sb.g, sb.b, 1)
  gl.Begin(gl.QUADS)
  gl.Vertex2i(int32(region.X+region.Dx/3), int32(region.Y))
  gl.Vertex2i(int32(region.X+region.Dx/3), int32(region.Y+region.Dy))
  gl.Vertex2i(int32(region.X+region.Dx/3*2), int32(region.Y+region.Dy))
  gl.Vertex2i(int32(region.X+region.Dx/3*2), int32(region.Y))
  gl.End()
  if sb.s != nil {
    gl.Enable(gl.TEXTURE_2D)
    tx, ty, tx2, ty2 := sb.s.Bind()
    // fmt.Printf("Tex: %f %f %f %f\n", tx, ty, tx2, ty2)
    gl.Enable(gl.BLEND)
    gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
    gl.Color4f(1, 1, 1, 1)
    gl.Begin(gl.QUADS)
    x := int32(region.X + region.Dx/2)
    y := int32(region.Y + region.Dy/2)
    gl.TexCoord2d(tx, -ty)
    gl.Vertex2i(x-50, y-75)
    gl.TexCoord2d(tx, -ty2)
    gl.Vertex2i(x-50, y+75)
    gl.TexCoord2d(tx2, -ty2)
    gl.Vertex2i(x+50, y+75)
    gl.TexCoord2d(tx2, -ty)
    gl.Vertex2i(x+50, y-75)
    gl.End()
    gl.Color4d(1, 1, 1, 1)
    text := fmt.Sprintf("%d : %s : %s", sb.s.Facing(), sb.s.Anim(), sb.s.AnimState())
    if sb.top {
      dict.RenderString(text, float64(region.X), float64(region.Y+region.Dy)-dict.MaxHeight(), 0, dict.MaxHeight(), gui.Left)
    } else {
      dict.RenderString(text, float64(region.X), float64(region.Y), 0, dict.MaxHeight(), gui.Left)
    }
  }
}

type boxdata struct {
  sb   *spriteBox
  name string
  dir  string
}

func (b *boxdata) load(dir string) {
  go func() {
    anim, err := sprite.LoadSprite(dir)
    if err != nil {
      error_msg.SetText(err.Error())
    } else {
      b.sb.s = anim
      SetStoreVal(b.name, b.dir)
    }
  }()

  // This is outside of the goroutine because otherwise it might never become
  // visible since we never sync up with it.
  b.dir = dir
}

type handler struct {
  box1, box2 *boxdata
}

func (h *handler) HandleEventGroup(group gin.EventGroup) {
  if group.Events[0].Type != gin.Press {
    return
  }
  if h.box1 == nil || h.box1.sb == nil || h.box1.sb.s == nil {
    return
  }
  for name, key := range action_map {
    if group.Events[0].Key.Id() == key.Id() {
      cmd := commands[name]
      if len(cmd.You) > 0 {
        if h.box2 == nil || h.box2.sb == nil || h.box2.sb.s == nil {
          return
        }
        sprite.CommandSync(
          []*sprite.Sprite{h.box1.sb.s, h.box2.sb.s},
          [][]string{cmd.Me, cmd.You},
          cmd.Sync)
      } else {
        h.box1.sb.s.CommandN(cmd.Me)
      }
    }
  }
}
func (h *handler) Think(int64) {}

func loadFont() (*truetype.Font, error) {
  f, err := os.Open(filepath.Join(datadir, "fonts", "luxisr.ttf"))
  if err != nil {
    return nil, err
  }
  data, err := ioutil.ReadAll(f)
  f.Close()
  if err != nil {
    return nil, err
  }
  font, err := truetype.Parse(data)
  if err != nil {
    return nil, err
  }
  return font, nil
}


func main() {
  sys = system.Make(gos.GetSystemInterface())
  sys.Startup()
  wdx := 1000
  wdy := 500
  render.Init()
  var ui *gui.Gui
  render.Queue(func() {
    sys.CreateWindow(50, 150, wdx, wdy)
    sys.EnableVSync(true)
    err := gl.Init()
    if err != nil {
      f, err2 := os.Create(filepath.Join(datadir, "gl_log.txt"))
      if err2 != nil {
        fmt.Printf("Unable to write log to a file:%v\n%v\v", err, err2)
      } else {
        fmt.Fprintf(f, "%v\n", err)
        f.Close()
      }
    }
    ui, _ = gui.Make(gin.In(), gui.Dims{wdx, wdy}, filepath.Join(datadir, "fonts", "luxisr.ttf"))
    font, err := loadFont()
    if err != nil {
      panic(err.Error())
    }
    dict = gui.MakeDictionary(font, 15)
  })
  render.Purge()

  anchor := gui.MakeAnchorBox(gui.Dims{wdx, wdy})
  ui.AddChild(anchor)
  var event_handler handler
  gin.In().RegisterEventListener(&event_handler)
  actions_list := gui.MakeVerticalTable()
  keyname_list := gui.MakeVerticalTable()
  both_lists := gui.MakeHorizontalTable()
  both_lists.AddChild(actions_list)
  both_lists.AddChild(keyname_list)
  anchor.AddChild(both_lists, gui.Anchor{1, 0.5, 1, 0.5})
  var actions []string
  for action := range action_map {
    actions = append(actions, action)
  }
  sort.Strings(actions)
  for _, action := range actions {
    actions_list.AddChild(gui.MakeTextLine("standard", action, 150, 1, 1, 1, 1))
    keyname_list.AddChild(gui.MakeTextLine("standard", commands[action].Cmd, 100, 1, 1, 1, 1))
  }

  current_anim := gui.MakeTextLine("standard", "", 300, 1, 1, 1, 1)
  current_state := gui.MakeTextLine("standard", "", 300, 1, 1, 1, 1)
  frame_data := gui.MakeVerticalTable()
  frame_data.AddChild(current_anim)
  frame_data.AddChild(current_state)
  anchor.AddChild(frame_data, gui.Anchor{0, 1, 0, 1})

  speed := 100
  speed_text := gui.MakeTextLine("standard", "Speed: 100%", 150, 1, 1, 1, 1)
  anchor.AddChild(speed_text, gui.Anchor{0, 0, 0, 0})

  var box1, box2 boxdata

  box1.name = "box1"
  box1.sb = makeSpriteBox(nil)
  anchor.AddChild(box1.sb, gui.Anchor{0.5, 0.5, 0.25, 0.5})
  box1.load(GetStoreVal("box1"))
  box := box1

  box2.name = "box2"
  box2.sb = makeSpriteBox(nil)
  anchor.AddChild(box2.sb, gui.Anchor{0.5, 0.5, 0.45, 0.5})
  box2.load(GetStoreVal("box2"))
  box2.sb.top = true
  box_other := box2

  box2.sb.r, box2.sb.g, box2.sb.b = 0.2, 0.1, 0.4
  box1.sb.r, box1.sb.g, box1.sb.b = 0.4, 0.2, 0.8

  error_msg = gui.MakeTextLine("standard", "", wdx, 1, 0.5, 0.5, 1)
  anchor.AddChild(error_msg, gui.Anchor{0, 0, 0, 0.1})

  var chooser gui.Widget
  // curdir := GetStoreVal("curdir")
  // if curdir == "" {
  //   curdir = "."
  // } else {
  //   _,err := os.Stat(filepath.Join(datadir, curdir))
  //   if err == nil {
  //     go func() {
  //       anim, err := sprite.LoadSprite(filepath.Join(datadir, curdir))
  //       loaded <- loadResult{ anim, err } 
  //     } ()
  //   } else {
  //     curdir = "."
  //   }
  // }
  // var profile_output *os.File
  then := time.Now()
  sys.Think()
  for key_map["quit"].FramePressCount() == 0 {
    event_handler.box1 = &box
    event_handler.box2 = &box_other
    now := time.Now()
    dt := (now.Nanosecond() - then.Nanosecond()) / 1000000
    then = now
    render.Queue(func() {
      sys.Think()
    if box1.sb.s != nil {
      box1.sb.s.Think(int64(float64(dt) * float64(speed) / 100))
    }
    if box2.sb.s != nil {
      box2.sb.s.Think(int64(float64(dt) * float64(speed) / 100))
    }
      gl.ClearColor(1, 0, 0, 1)
      gl.Clear(gl.COLOR_BUFFER_BIT)
      ui.Draw()
      sys.SwapBuffers()
    })
    render.Purge()
    select {
    case load := <-loaded:
      if load.err != nil {
        error_msg.SetText(load.err.Error())
        current_anim.SetText("")
      } else {
        box.sb.s = load.anim
        error_msg.SetText("")
      }
    default:
    }
    // if box.sb.s != nil {
    //   box.sb.s.Think()
    //   current_anim.SetText(fmt.Sprintf("%d: %s", box.sb.s.Facing(), box.sb.s.Anim()))
    //   current_state.SetText(box.sb.s.AnimState())
    // }

    if box.sb.s != nil {
      if key_map["reset"].FramePressCount() > 0 {
        box.load(box.dir)
        box_other.load(box_other.dir)
      }
    }

    // if key_map["profile"].FramePressCount() > 0 {
    //   if profile_output == nil {
    //     var err error
    //     profile_output, err = os.Create(filepath.Join(datadir, "cpu.prof"))
    //     if err == nil {
    //       err = pprof.StartCPUProfile(profile_output)
    //       if err != nil {
    //         fmt.Printf("Unable to start CPU profile: %v\n", err)
    //         profile_output.Close()
    //         profile_output = nil
    //       }
    //       fmt.Printf("profout: %v\n", profile_output)
    //     } else {
    //       fmt.Printf("Unable to open CPU profile: %v\n", err)
    //     }
    //   } else {
    //     pprof.StopCPUProfile()
    //     profile_output.Close()
    //     profile_output = nil
    //   }
    // }

    if key_map["load"].FramePressCount() > 0 && chooser == nil {
      anch := gui.MakeAnchorBox(gui.Dims{wdx, wdy})
      file_chooser := gui.MakeFileChooser(filepath.Join(datadir, box.dir),
        func(path string, err error) {
          if err == nil && len(path) > 0 {
            curpath, _ := filepath.Split(path)
            box.load(curpath)
          }
          ui.RemoveChild(chooser)
          chooser = nil
        },
        func(path string, is_dir bool) bool {
          return true
        })
      anch.AddChild(file_chooser, gui.Anchor{0.5, 0.5, 0.5, 0.5})
      chooser = anch
      ui.AddChild(chooser)
    }
    delta := key_map["speed up"].FramePressAmt() - key_map["slow down"].FramePressAmt()
    if delta != 0 {
      speed += int(delta)
      if speed < 1 {
        speed = 1
      }
      if speed > 100 {
        speed = 100
      }
      speed_text.SetText(fmt.Sprintf("Speed: %d%%", speed))
    }

    if key_map["select1"].FramePressCount() > 0 {
      box2.sb.r, box2.sb.g, box2.sb.b = 0.2, 0.1, 0.4
      box1.sb.r, box1.sb.g, box1.sb.b = 0.4, 0.2, 0.8
      box = box1
      box_other = box2
    }
    if key_map["select2"].FramePressCount() > 0 {
      box2.sb.r, box2.sb.g, box2.sb.b = 0.4, 0.2, 0.8
      box1.sb.r, box1.sb.g, box1.sb.b = 0.2, 0.1, 0.4
      box = box2
      box_other = box1
    }
  }
}
