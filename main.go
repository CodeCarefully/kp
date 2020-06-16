package main

import (
	"flag"
	"fmt"
	"github.com/abiosoft/ishell"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"zombiezen.com/go/sandpass/pkg/keepass"
)

var (
	keyFile = flag.String("key", "", "a key file to use to unlock the db")
	dbFile  = flag.String("db", "", "the db to open")
)

func main() {
	flag.Parse()

	if *dbFile == "" {
		log.Fatalf("no db file provided!")
	}

	dbReader, err := os.Open(*dbFile)
	if err != nil {
		log.Fatalf("could not open db file [%s]: %s", *dbFile, err)
	}

	var keyReader io.Reader
	if *keyFile != "" {
		keyReader, err = os.Open(*keyFile)
		if err != nil {
			log.Fatalf("could not open key file %s", *keyFile)
		}
	}
	shell := ishell.New()

	shell.Println("enter database password")
	password, err := shell.ReadPasswordErr()
	if err != nil {
		log.Fatalf("could not obtain password: %s", password)
	}

	opts := &keepass.Options{
		Password: password,
		KeyFile:  keyReader,
	}

	db, err := keepass.Open(dbReader, opts)
	if err != nil {
		log.Fatalf("could not open database [%s]: %s", *dbFile, err)
	}

	shell.Println("opened database")
	shell.Set("currentLocation", db.Root())
	shell.SetPrompt(fmt.Sprintf("%s > ", db.Root().Name))
	shell.AddCmd(&ishell.Cmd{
		Name: "ls",
		Help: "ls [path]",
		Func: func(c *ishell.Context) {
			currentLocation := c.Get("currentLocation").(*keepass.Group)
			location := currentLocation
			if len(c.Args) > 0 {
				newLocation, err := traversePath(currentLocation, db.Root(), c.Args[0])
				if err != nil {
					c.Err(fmt.Errorf("Invalid path: %s", err))
					return
				}
				location = newLocation
			}

			lines := []string{}
			for _, group := range location.Groups() {
				lines = append(lines, fmt.Sprintf("%s/", group.Name))
			}
			for i, entry := range location.Entries() {
				lines = append(lines, fmt.Sprintf("%d: %s", i, entry.Title))
			}
			c.Println(strings.Join(lines, "\n"))
		},
	})
	shell.AddCmd(&ishell.Cmd{
		Name: "show",
		Help: "show [-f] <entry>",
		Func: func(c *ishell.Context) {
			if len(c.Args) < 1 {
				c.Err(fmt.Errorf("incorrect number of arguments to show"))
				return
			}

			fullMode := false
			entryName := c.Args[0]
			for _, arg := range c.Args {
				if strings.HasPrefix(arg, "-") {
					if arg == "-f" {
						fullMode = true
					}
					continue
				}
				entryName = arg
			}

			currentLocation := c.Get("currentLocation").(*keepass.Group)
			location, err := traversePath(currentLocation, db.Root(), entryName)
			if err != nil {
				c.Err(fmt.Errorf("could not find entry named [%s]", entryName))
				return
			}

			for i, entry := range location.Entries() {
				if intVersion, err := strconv.Atoi(entryName); err == nil && intVersion == i {
					outputEntry(*entry, c, fullMode)
					break
				}

				if entryName == entry.Title {
					outputEntry(*entry, c, fullMode)
					break
				}
			}
		},
	})
	shell.AddCmd(&ishell.Cmd{
		Name: "cd",
		Help: "cd <path>",
		Func: func(c *ishell.Context) {
			args := c.Args
			currentLocation := c.Get("currentLocation").(*keepass.Group)
			if len(args) == 0 {
				currentLocation = db.Root()
			} else {
				newLocation, err := traversePath(currentLocation, db.Root(), args[0])
				if err != nil {
					c.Err(fmt.Errorf("invalid path: %s", err))
					return
				}
				currentLocation = newLocation
			}
			shell.Set("currentLocation", currentLocation)
			c.SetPrompt(fmt.Sprintf("%s > ", currentLocation.Name))
		}})
	shell.Run()
}

func outputEntry(e keepass.Entry, c *ishell.Context, full bool) {
	c.Println(fmt.Sprintf("Title: %s", e.Title))
	c.Println(fmt.Sprintf("URL: %s", e.URL))
	c.Println(fmt.Sprintf("Username: %s", e.URL))
	password := "[redacted]"
	if full {
		password = e.Password
	}
	c.Println(fmt.Sprintf("Password: %s", password))
	c.Println(fmt.Sprintf("Notes : %s", e.Notes))
	if e.HasAttachment() {
		c.Println(fmt.Sprintf("Attachment: %s", e.Attachment.Name))
	}

}

// given a starting location and a UNIX-style path, will walk the path and return the final location or an error
func traversePath(startingLocation *keepass.Group, root *keepass.Group, fullPath string) (finalLocation *keepass.Group, err error) {
	currentLocation := startingLocation
	if fullPath == "/" {
		// short circuit now
		return root, nil
	}

	if strings.HasPrefix(fullPath, "/") {
		// the user entered a fully qualified path, so start at the top
		currentLocation = root
	}

	// break the path up into components
	path := strings.Split(fullPath, "/")
	for _, part := range path {
		if part == "." || part == "" {
			continue
		}

		if part == ".." {
			if currentLocation.Parent() != nil {
				currentLocation = currentLocation.Parent()
				continue
			}
			return nil, fmt.Errorf("root group has no parent")
		}
		// regular traversal
		found := false
		for _, group := range currentLocation.Groups() {
			if group.Name == part {
				currentLocation = group
				found = true
				break
			}
		}
		for i, entry := range currentLocation.Entries() {
			if entry.Title == part || strconv.Itoa(i) == part {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("could not find a group or entry named [%s]", part)
		}
	}
	return currentLocation, nil
}
