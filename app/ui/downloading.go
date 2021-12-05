package ui

import (
	"archive/zip"
	"fmt"
	"github.com/darylhjd/mangadesk/app/core"
	"io"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/darylhjd/mangodex"
	"github.com/rivo/tview"
)

// downloadChapters : Download current chapters specified by the user.
func (p *MangaPage) downloadChapters() {
	// Get a reference to the current selection
	selection := p.Selected
	// Reset.
	p.Selected = map[int]struct{}{}

	// Clear selection highlight
	for index := range selection {
		markChapterUnselected(p.Table, index)
	}

	// Download the selected chapters.
	var errored bool
	for index := range selection {
		// Get the reference to the chapter.
		chapter := p.Table.GetCell(index, 0).GetReference().(*mangodex.Chapter)

		// Save the current chapter.
		err := p.saveChapter(chapter)
		if err != nil {
			// If there was an error saving current chapter, we skip and continue trying next chapters.
			msg := fmt.Sprintf("Error saving %s - Chapter: %s, %s - %s",
				p.Manga.GetTitle("en"), chapter.GetChapterNum(), chapter.GetTitle(), err.Error())
			log.Println(msg)
			errored = true
			continue
		}

		core.App.TView.QueueUpdateDraw(func() {
			downloadCell := tview.NewTableCell("Y").SetTextColor(MangaPageDownloadStatColor)
			p.Table.SetCell(index, 2, downloadCell)
		})
	}

	var msg strings.Builder
	msg.WriteString("Last Download Queue finished.\n")
	if errored {
		msg.WriteString("We encountered some errors! Check the log for more details.")
	} else {
		msg.WriteString("No errors :>")
	}

	core.App.TView.QueueUpdateDraw(func() {
		// Create download finished modal.
		modal := okModal(DownloadFinishedModalID, msg.String())
		ShowModal(DownloadFinishedModalID, modal)
	})
}

// saveChapter : Save a chapter.
func (p *MangaPage) saveChapter(chapter *mangodex.Chapter) error {
	downloader, err := core.App.Client.AtHome.NewMDHomeClient(
		chapter, core.App.Config.DownloadQuality, core.App.Config.ForcePort443)
	if err != nil {
		return err
	}

	// Create directory to store the current chapter.
	downloadFolder := p.getDownloadFolder(chapter)
	if err = os.MkdirAll(downloadFolder, os.ModePerm); err != nil {
		return err
	}

	// Get the pages to download
	pages := chapter.Attributes.Data
	if core.App.Config.DownloadQuality == "data-saver" {
		pages = chapter.Attributes.DataSaver
	}

	// Save each page.
	for num, page := range pages {
		// Get image data.
		image, err := downloader.GetChapterPage(page)
		if err != nil {
			return err
		}

		filename := fmt.Sprintf("%04d%s", num+1, filepath.Ext(page))
		filePath := filepath.Join(downloadFolder, filename)
		// Save image
		if err = ioutil.WriteFile(filePath, image, os.ModePerm); err != nil {
			return err
		}
	}

	// If user wants to save the downloads as a zip, then do so.
	if core.App.Config.AsZip {
		if err = p.saveAsZipFolder(downloadFolder); err != nil {
			return err
		} else if err = os.RemoveAll(downloadFolder); err != nil {
			return err
		}
	}
	return nil
}

// saveAsZipFolder : This function creates a zip folder to store a chapter download.
func (p *MangaPage) saveAsZipFolder(chapterFolder string) error {
	zipFile, err := os.Create(fmt.Sprintf("%s.%s", chapterFolder, core.App.Config.ZipType))
	if err != nil {
		return err
	}
	defer func() {
		_ = zipFile.Close()
	}()

	w := zip.NewWriter(zipFile)
	defer func() {
		_ = w.Close()
	}()

	return filepath.WalkDir(chapterFolder, func(path string, d fs.DirEntry, err error) error {
		// Stop walking immediately if encounter error
		if err != nil {
			return err
		}
		// Skip if a DirEntry is a folder. By right, this shouldn't happen since any downloads will
		// just contain PNGs or JPEGs, but it's here just in case.
		if d.IsDir() {
			return nil
		}

		// Open the original image file.
		fileOriginal, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() {
			_ = fileOriginal.Close()
		}()

		// Create designated file in zip folder for current image.
		// Use custom header to set modified timing.
		// Fixes zip parsing issues in certain situations.
		fh := zip.FileHeader{
			Name:     d.Name(),
			Modified: time.Now(),
			Method:   zip.Deflate, // Consistent with w.Create() source code.
		}
		fileZip, err := w.CreateHeader(&fh)
		if err != nil {
			return err
		}

		// Copy the original file into its designated file in the zip archive.
		_, err = io.Copy(fileZip, fileOriginal)
		if err != nil {
			return err
		}
		return nil
	})
}

// getDownloadFolder : Get the download folder for a manga's chapter.
func (p *MangaPage) getDownloadFolder(chapter *mangodex.Chapter) string {
	mangaName := p.Manga.GetTitle("en")
	chapterName := fmt.Sprintf("Chapter%s [%s-%s] %s_%s",
		chapter.GetChapterNum(), strings.ToUpper(chapter.Attributes.TranslatedLanguage), core.App.Config.DownloadQuality,
		chapter.GetTitle(), strings.SplitN(chapter.ID, "-", 2)[0])

	// Remove invalid characters from the folder name
	restricted := []string{"<", ">", ":", "/", "|", "?", "*", "\"", "\\", "."}
	for _, c := range restricted {
		mangaName = strings.ReplaceAll(mangaName, c, "")
		chapterName = strings.ReplaceAll(chapterName, c, "")
	}

	folder := filepath.Join(core.App.Config.DownloadDir, mangaName, chapterName)
	// If the user wants to download as a zip, then we check for the presence of the zip folder.
	if core.App.Config.AsZip {
		folder = fmt.Sprintf("%s.%s", folder, core.App.Config.ZipType)
	}
	return folder
}
