using Nikse.SubtitleEdit.Core.BluRaySup;
using PgsToSrt.BluRaySup;
using SixLabors.ImageSharp;
using Xunit;

namespace PgsToSrt.RegressionTests;

public class DecoderBoundsTests
{
    [Fact]
    public void GetRgba32_ClipsRunLengthBeyondBitmapBounds()
    {
        var subtitle = new BluRaySupParserImageSharp.PcsData
        {
            PcsObjects = [new BluRaySupParserImageSharp.PcsObject
            {
                Origin = new Point(0, 0),
            }],
            BitmapObjects = [[new BluRaySupParserImageSharp.OdsData
            {
                Size = new Size(1, 1),
                Fragment = new ImageObjectFragment
                {
                    // A 255-pixel run is invalid for the declared one-pixel bitmap.
                    ImageBuffer = [0, 192, 0, 255, 0],
                },
            }]],
            PaletteInfos = [],
        };

        using var image = subtitle.GetRgba32();

        Assert.Equal(51, image.Width);
        Assert.Equal(51, image.Height);
    }
}
